package fs

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jstaf/onedriver/fs/conflict"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	bolt "go.etcd.io/bbolt"
)

// DeltaLoop creates a new thread to poll the server for changes and should be
// called as a goroutine
func (f *Filesystem) DeltaLoop(interval time.Duration) {
	log.Trace().Msg("Starting delta goroutine.")
	for { // eva
		// get deltas
		log.Trace().Msg("Fetching deltas from server.")
		pollSuccess := false
		deltas := make(map[string]*graph.DriveItem)
		for {
			incoming, cont, err := f.pollDeltas(f.auth)
			if err != nil {
				// the only thing that should be able to bring the FS out
				// of a read-only state is a successful delta call
				log.Error().Err(err).
					Msg("Error during delta fetch, marking fs as offline.")
				f.Lock()
				f.offline = true
				f.Unlock()
				break
			}

			for _, delta := range incoming {
				// As per the API docs, the last delta received from the server
				// for an item is the one we should use.
				deltas[delta.ID] = delta
			}
			if !cont {
				log.Info().Msgf("Fetched %d deltas.", len(deltas))
				pollSuccess = true
				break
			}
		}

		// now apply deltas
		secondPass := make([]string, 0)
		for _, delta := range deltas {
			err := f.applyDelta(delta)
			// retry deletion of non-empty directories after all other deltas applied
			if err != nil && err.Error() == "directory is non-empty" {
				secondPass = append(secondPass, delta.ID)
			}
		}
		for _, id := range secondPass {
			// failures should explicitly be ignored the second time around as per docs
			f.applyDelta(deltas[id])
		}

		if !f.IsOffline() {
			f.SerializeAll()
		}

		if pollSuccess {
			f.Lock()
			if f.offline {
				log.Info().Msg("Delta fetch success, marking fs as online.")
			}
			f.offline = false
			f.Unlock()

			f.db.Batch(func(tx *bolt.Tx) error {
				return tx.Bucket(bucketDelta).Put([]byte("deltaLink"), []byte(f.deltaLink))
			})

			// wait until next interval
			time.Sleep(interval)
		} else {
			// shortened duration while offline
			time.Sleep(2 * time.Second)
		}
	}
}

type deltaResponse struct {
	NextLink  string             `json:"@odata.nextLink,omitempty"`
	DeltaLink string             `json:"@odata.deltaLink,omitempty"`
	Values    []*graph.DriveItem `json:"value,omitempty"`
}

// Polls the delta endpoint and return deltas + whether or not to continue
// polling. Does not perform deduplication. Note that changes from the local
// client will actually appear as deltas from the server (there is no
// distinction between local and remote changes from the server's perspective,
// everything is a delta, regardless of where it came from).
func (f *Filesystem) pollDeltas(auth *graph.Auth) ([]*graph.DriveItem, bool, error) {
	resp, err := graph.Get(f.deltaLink, auth)
	if err != nil {
		return make([]*graph.DriveItem, 0), false, err
	}

	page := deltaResponse{}
	json.Unmarshal(resp, &page)

	// If the server does not provide a `@odata.nextLink` item, it means we've
	// reached the end of this polling cycle and should not continue until the
	// next poll interval.
	if page.NextLink != "" {
		f.deltaLink = strings.TrimPrefix(page.NextLink, graph.GraphURL)
		return page.Values, true, nil
	}
	f.deltaLink = strings.TrimPrefix(page.DeltaLink, graph.GraphURL)
	return page.Values, false, nil
}

// applyDelta diagnoses and applies a server-side change to our local state.
// Things we care about (present in the local cache):
// * Deleted items
// * Changed content remotely, but not locally
// * New items in a folder we have locally
func (f *Filesystem) applyDelta(delta *graph.DriveItem) error {
	id := delta.ID
	name := delta.Name
	parentID := delta.Parent.ID
	ctx := log.With().
		Str("id", id).
		Str("parentID", parentID).
		Str("name", name).
		Logger()
	ctx.Debug().Msg("Applying delta")

	// diagnose and act on what type of delta we're dealing with

	// do we have it at all?
	if parent := f.GetID(parentID); parent == nil {
		// Nothing needs to be applied, item not in cache, so latest copy will
		// be pulled down next time it's accessed.
		ctx.Trace().
			Str("delta", "skip").
			Msg("Skipping delta, item's parent not in cache.")
		return nil
	}

	local := f.GetID(id)

	// was it deleted?
	if delta.Deleted != nil {
		if delta.IsDir() && local != nil && local.HasChildren() {
			// from docs: you should only delete a folder locally if it is empty
			// after syncing all the changes.
			ctx.Warn().Str("delta", "delete").
				Msg("Refusing delta deletion of non-empty folder as per API docs.")
			return errors.New("directory is non-empty")
		}
		ctx.Info().Str("delta", "delete").
			Msg("Applying server-side deletion of item.")
		f.DeleteID(id)
		return nil
	}

	// does the item exist locally? if not, add the delta to the cache under the
	// appropriate parent
	prevID := "" // local ID the item was tracked under before this delta, if any
	if local == nil {
		// check if we don't have it here first
		local, _ = f.GetChild(parentID, name, nil)
		if local != nil {
			localID := local.ID()
			ctx.Info().
				Str("localID", localID).
				Msg("Local item already exists under different ID.")
			if isLocalID(localID) {
				prevID = localID
				if err := f.MoveID(localID, id); err != nil {
					ctx.Error().
						Str("localID", localID).
						Err(err).
						Msg("Could not move item to new, nonlocal ID!")
				}
			}
		} else {
			ctx.Info().Str("delta", "create").
				Msg("Creating inode from delta.")
			f.InsertChild(parentID, NewInodeDriveItem(delta))
			return nil
		}
	}

	// was the item moved?
	localName := local.Name()
	if local.ParentID() != parentID || local.Name() != name {
		log.Info().
			Str("parent", local.ParentID()).
			Str("name", localName).
			Str("newParent", parentID).
			Str("newName", name).
			Str("id", id).
			Str("delta", "rename").
			Msg("Applying server-side rename")
		oldParentID := local.ParentID()
		// local rename only
		f.MovePath(oldParentID, parentID, localName, name, f.auth)
		// do not return, there may be additional changes
	}

	// Finally, check if the content/metadata of the remote has changed.
	// "Interesting" changes must be synced back to our local state without
	// data loss or corruption. Currently the only thing the local filesystem
	// actually modifies remotely is the actual file data, so we simply accept
	// the remote metadata changes that do not deal with the file's content
	// changing.
	local.RLock()
	localETag := local.DriveItem.ETag
	localModTime := local.DriveItem.ModTimeUnix()
	local.RUnlock()
	if delta.ETagIsMatch(localETag) {
		ctx.Trace().Str("delta", "skip").Msg("Skipping, no changes relative to local state.")
		return nil
	}

	if delta.IsDir() {
		// folders have no content to lose, just accept newer remote metadata
		if delta.ModTimeUnix() > localModTime {
			ctx.Info().Str("delta", "overwrite").
				Msg("Overwriting local folder metadata with server version.")
			local.Lock()
			local.DriveItem.ModTime = delta.ModTime
			local.DriveItem.Size = delta.Size
			local.DriveItem.ETag = delta.ETag
			local.Unlock()
			return nil
		}
		ctx.Trace().Str("delta", "skip").Msg("Skipping, no changes relative to local state.")
		return nil
	}

	// Files: content hashes, not modification times, decide what happens. The
	// local copy may have pending changes that must never be overwritten.
	ids := []string{id, prevID}
	remoteHash := ""
	if delta.File != nil {
		remoteHash = delta.File.Hashes.QuickXorHash
	}
	local.RLock()
	localHash := ""
	if local.DriveItem.File != nil {
		localHash = local.DriveItem.File.Hashes.QuickXorHash
	}
	localChanges := local.hasChanges
	local.RUnlock()
	localChanges = localChanges || f.uploads.IsUploadPending(ids...)

	action := conflict.Resolve(remoteHash, localHash, localChanges, f.uploads.UploadedHashes(ids...))
	if action == conflict.Overwrite && !f.overwriteFromDelta(local, delta, ids, ctx) {
		// local was modified while we were deciding, re-evaluate as changed
		action = conflict.Resolve(remoteHash, localHash, true, f.uploads.UploadedHashes(ids...))
	}

	switch action {
	case conflict.Overwrite:
		return nil
	case conflict.Conflict:
		ctx.Warn().
			Str("delta", "conflict").
			Str("localHash", localHash).
			Str("remoteHash", remoteHash).
			Msg("Conflict detected: item changed on server while local changes " +
				"are not yet uploaded. Keeping local version, saving server " +
				"version as a conflict copy.")
		if err := f.createConflictCopy(delta, ctx); err != nil {
			ctx.Error().Err(err).Str("delta", "conflict").
				Msg("Could not create conflict copy of server version! The " +
					"local version will still be uploaded, the overwritten server " +
					"version can only be recovered from OneDrive's version history.")
			return err
		}
		// remember that we have seen (and preserved) this server version
		local.Lock()
		local.DriveItem.ETag = delta.ETag
		local.Unlock()
		return nil
	case conflict.Unknown:
		ctx.Warn().Str("delta", "conflict").
			Msg("Item changed on server but has no content hash to compare " +
				"against, keeping local changes and not overwriting them.")
		return nil
	}

	ctx.Trace().Str("delta", "skip").Msg("Skipping, no changes relative to local state.")
	return nil
}

// overwriteFromDelta replaces a local file's metadata with the server's and
// invalidates its cached content so the new version gets downloaded on next
// open. Returns false (and changes nothing) if the local item has changes or
// a pending upload, which is re-checked here under the inode lock.
func (f *Filesystem) overwriteFromDelta(local *Inode, delta *graph.DriveItem, ids []string, ctx zerolog.Logger) bool {
	local.Lock()
	defer local.Unlock()
	// lock order: inode -> upload manager (the upload manager never takes
	// inode locks while holding its own)
	if local.hasChanges || f.uploads.IsUploadPending(ids...) {
		return false
	}
	ctx.Info().Str("delta", "overwrite").
		Msg("Overwriting local item, no local changes to preserve.")
	local.DriveItem.ModTime = delta.ModTime
	local.DriveItem.Size = delta.Size
	local.DriveItem.ETag = delta.ETag
	local.DriveItem.File = delta.File
	// Open() would also notice the hash mismatch and re-download, but drop the
	// stale content now. Files that are currently open keep their fd and get
	// refreshed by the checksum check on the next Open().
	if id := local.DriveItem.ID; !f.content.IsOpen(id) && f.content.HasContent(id) {
		ctx.Debug().Str("delta", "overwrite").Msg("Invalidating cached content.")
		f.content.Delete(id)
	}
	return true
}

// createConflictCopy preserves the server version of a file as a new local
// item under a conflict name (which is then uploaded). The original item keeps
// its ID and local content, so its pending upload will replace the server
// version - upload sessions do not send an If-Match header and use
// conflictBehavior=replace, so this cannot fail on an eTag mismatch.
func (f *Filesystem) createConflictCopy(delta *graph.DriveItem, ctx zerolog.Logger) error {
	if delta.File == nil || delta.File.Hashes.QuickXorHash == "" {
		return errors.New("server item has no content hash")
	}
	parentID := delta.Parent.ID

	// pick a name that is not already taken in the parent folder
	host := conflict.Hostname()
	now := time.Now()
	var name string
	for n := 1; ; n++ {
		name = conflict.Name(delta.Name, host, now, n)
		if child, _ := f.GetChild(parentID, name, f.auth); child == nil {
			break
		}
		if n >= 100 {
			return errors.New("could not find a free conflict name")
		}
	}

	item := *delta
	item.ID = localID()
	item.Name = name
	item.ETag = ""
	item.Deleted = nil
	parent := *delta.Parent
	item.Parent = &parent
	file := *delta.File
	item.File = &file
	if item.ModTime == nil {
		item.ModTime = &now
	}
	copyInode := NewInodeDriveItem(&item)

	// download the server version into the cache under the new ID
	fd, err := f.content.Open(item.ID)
	if err != nil {
		return err
	}
	size, err := graph.GetItemContentStream(delta.ID, f.auth, fd)
	if err != nil {
		f.content.Delete(item.ID)
		return err
	}
	if !copyInode.VerifyChecksum(graph.QuickXORHashStream(fd)) {
		// most likely the server copy changed again since the delta (possibly
		// our own upload landed first)
		f.content.Delete(item.ID)
		return errors.New("downloaded server version did not match delta checksum")
	}
	f.content.Close(item.ID)
	copyInode.DriveItem.Size = size

	f.InsertChild(parentID, copyInode)
	ctx.Warn().
		Str("delta", "conflict").
		Str("conflictName", name).
		Str("conflictID", item.ID).
		Msg("Saved server version as conflict copy, uploading it.")
	return f.uploads.QueueUpload(copyInode)
}
