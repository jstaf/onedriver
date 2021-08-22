package fs

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

// DeltaLoop creates a new thread to poll the server for changes and should be
// called as a goroutine
func (c *Cache) DeltaLoop(interval time.Duration) {
	log.Trace("Starting delta goroutine.")
	for { // eva
		// get deltas
		log.Debug("Fetching deltas from server.")
		pollSuccess := false
		deltas := make(map[string]*Inode)
		for {
			incoming, cont, err := c.pollDeltas(c.GetAuth())
			if err != nil {
				// the only thing that should be able to bring the FS out
				// of a read-only state is a successful delta call
				log.WithField("err", err).Error(
					"Error during delta fetch, marking fs as offline.",
				)
				c.Lock()
				c.offline = true
				c.Unlock()
				break
			}

			for _, delta := range incoming {
				// As per the API docs, the last delta received from the server
				// for an item is the one we should use.
				deltas[delta.ID()] = delta
			}
			if !cont {
				log.Infof("Fetched %d deltas.", len(deltas))
				pollSuccess = true
				break
			}
		}

		// now apply deltas
		secondPass := make([]string, 0)
		for _, delta := range deltas {
			err := c.applyDelta(delta)
			// retry deletion of non-empty directories after all other deltas applied
			if err != nil && err.Error() == "directory is non-empty" {
				secondPass = append(secondPass, delta.ID())
			}
		}
		for _, id := range secondPass {
			// failures should explicitly be ignored the second time around as per docs
			c.applyDelta(deltas[id])
		}

		if !c.IsOffline() {
			c.SerializeAll()
		}

		if pollSuccess {
			c.Lock()
			if c.offline {
				log.Info("Delta fetch success, marking fs as online.")
			}
			c.offline = false
			c.Unlock()

			c.db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(bucketDelta).Put([]byte("deltaLink"), []byte(c.deltaLink))
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
	NextLink  string   `json:"@odata.nextLink,omitempty"`
	DeltaLink string   `json:"@odata.deltaLink,omitempty"`
	Values    []*Inode `json:"value,omitempty"`
}

// Polls the delta endpoint and return deltas + whether or not to continue
// polling. Does not perform deduplication. Note that changes from the local
// client will actually appear as deltas from the server (there is no
// distinction between local and remote changes from the server's perspective,
// everything is a delta, regardless of where it came from).
func (c *Cache) pollDeltas(auth *graph.Auth) ([]*Inode, bool, error) {
	resp, err := graph.Get(c.deltaLink, auth)
	if err != nil {
		return make([]*Inode, 0), false, err
	}

	page := deltaResponse{}
	json.Unmarshal(resp, &page)

	// If the server does not provide a `@odata.nextLink` item, it means we've
	// reached the end of this polling cycle and should not continue until the
	// next poll interval.
	if page.NextLink != "" {
		c.deltaLink = strings.TrimPrefix(page.NextLink, graph.GraphURL)
		return page.Values, true, nil
	}
	c.deltaLink = strings.TrimPrefix(page.DeltaLink, graph.GraphURL)
	return page.Values, false, nil
}

// applyDelta diagnoses and applies a server-side change to our local state.
// Things we care about (present in the local cache):
// * Deleted items
// * Changed content remotely, but not locally
// * New items in a folder we have locally
func (c *Cache) applyDelta(delta *Inode) error {
	id := delta.ID()
	name := delta.Name()
	log.WithFields(log.Fields{
		"id":   id,
		"name": name,
	}).Debug("Applying delta")

	// diagnose and act on what type of delta we're dealing with

	// do we have it at all?
	parentID := delta.ParentID()
	if parent := c.GetID(parentID); parent == nil {
		// Nothing needs to be applied, item not in cache, so latest copy will
		// be pulled down next time it's accessed.
		log.WithFields(log.Fields{
			"id":       id,
			"parentID": parentID,
			"name":     name,
			"delta":    "skip",
		}).Trace("Skipping delta, item's parent not in cache.")
		return nil
	}

	local := c.GetID(id)

	// was it deleted?
	if delta.Deleted != nil {
		if delta.IsDir() && local != nil && local.HasChildren() {
			// from docs: you should only delete a folder locally if it is empty
			// after syncing all the changes.
			log.WithFields(log.Fields{
				"id":    id,
				"name":  name,
				"delta": "delete",
			}).Warn("Refusing delta deletion of non-empty folder as per API docs.")
			return errors.New("directory is non-empty")
		}
		log.WithFields(log.Fields{
			"id":    id,
			"name":  name,
			"delta": "delete",
		}).Info("Applying server-side deletion of item.")
		c.DeleteID(id)
		return nil
	}

	// does the item exist locally? if not, add the delta to the cache under the
	// appropriate parent
	if local == nil {
		// check if we don't have it here first
		local, _ = c.GetChild(parentID, name, nil)
		if local != nil {
			localID := local.ID()
			log.WithFields(log.Fields{
				"id":       id,
				"localID":  localID,
				"parentID": parentID,
				"name":     name,
			}).Info("Local item already exists under different ID.")
			if isLocalID(localID) {
				if err := c.MoveID(localID, id); err != nil {
					log.WithError(err).WithFields(log.Fields{
						"id":       id,
						"localID":  localID,
						"parentID": parentID,
						"name":     name,
					}).Error("Could not move item to new, nonlocal ID!")
				}
			}
		} else {
			log.WithFields(log.Fields{
				"id":       id,
				"parentID": parentID,
				"name":     name,
				"delta":    "create",
			}).Info("Creating inode from delta.")
			c.InsertChild(parentID, delta)
			return nil
		}
	}

	// was the item moved?
	localName := local.Name()
	if local.ParentID() != parentID || local.Name() != name {
		log.WithFields(log.Fields{
			"parent":    local.ParentID(),
			"name":      localName,
			"newParent": parentID,
			"newName":   name,
			"id":        id,
			"delta":     "rename",
		}).Info("Applying server-side rename")
		oldParentID := local.ParentID()
		// local rename only
		c.MovePath(oldParentID, parentID, localName, name, c.auth)
		// do not return, there may be additional changes
	}

	// Finally, check if the content/metadata of the remote has changed.
	// "Interesting" changes must be synced back to our local state without
	// data loss or corruption. Currently the only thing the local filesystem
	// actually modifies remotely is the actual file data, so we simply accept
	// the remote metadata changes that do not deal with the file's content
	// changing.
	if delta.ModTime() > local.ModTime() && !delta.ETagIsMatch(local.ETag) {
		sameContent := false
		if !delta.IsDir() && delta.File != nil {
			local.mutex.RLock()
			if delta.DriveItem.Parent.DriveType == graph.DriveTypePersonal {
				sameContent = local.VerifyChecksum(delta.File.Hashes.SHA1Hash)
			} else {
				sameContent = local.VerifyChecksum(delta.File.Hashes.QuickXorHash)
			}
			local.mutex.RUnlock()
		}

		if !sameContent {
			//TODO check if local has changes and rename the server copy if so
			log.WithFields(log.Fields{
				"id":    id,
				"name":  name,
				"delta": "overwrite",
			}).Info("Overwriting local item, no local changes to preserve.")
			// update modtime, hashes, purge any local content in memory
			local.mutex.Lock()
			defer local.mutex.Unlock()
			local.DriveItem.ModTime = delta.DriveItem.ModTime
			local.DriveItem.Size = delta.DriveItem.Size
			local.DriveItem.ETag = delta.DriveItem.ETag
			// the rest of these are harmless when this is a directory
			// as they will be null anyways
			local.DriveItem.File = delta.DriveItem.File
			local.hasChanges = false
			local.data = nil
			return nil
		}
	}

	log.WithFields(log.Fields{
		"id":    id,
		"name":  name,
		"delta": "skip",
	}).Trace("Skipping, no changes relative to local state.")
	return nil
}
