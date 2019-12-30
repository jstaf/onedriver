package graph

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// deltaLoop should be called as a goroutine
func (c *Cache) deltaLoop(interval time.Duration) {
	log.Trace("Starting delta goroutine.")
	for { // eva
		// get deltas
		log.Debug("Fetching deltas from server.")
		deltas := make(map[string]*Inode)
		for {
			incoming, cont, err := c.pollDeltas(c.auth)
			if err != nil {
				// TODO the only thing that should be able to bring the FS out
				// of a read-only state is a successful delta call
				log.WithField("err", err).Error("Error during delta fetch.")
				break
			}

			for _, delta := range incoming {
				// As per the API docs, the last delta received from the server
				// for an item is the one we should use.
				deltas[delta.ID()] = delta
			}
			if !cont {
				log.Infof("Fetched %d deltas.", len(deltas))
				break
			}
		}

		// now apply deltas
		for _, delta := range deltas {
			c.applyDelta(delta)
		}

		// sleep till next sync interval
		log.Info("Sync complete!")
		c.SerializeAll()
		time.Sleep(interval)
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
func (c *Cache) pollDeltas(auth *Auth) ([]*Inode, bool, error) {
	resp, err := Get(c.deltaLink, auth)
	if err != nil {
		return make([]*Inode, 0), false, err
	}

	page := deltaResponse{}
	json.Unmarshal(resp, &page)

	// If the server does not provide a `@odata.nextLink` item, it means we've
	// reached the end of this polling cycle and should not continue until the
	// next poll interval.
	if page.NextLink != "" {
		c.deltaLink = strings.TrimPrefix(page.NextLink, graphURL)
		return page.Values, true, nil
	}
	c.deltaLink = strings.TrimPrefix(page.DeltaLink, graphURL)
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

	// was it deleted?
	if delta.Deleted != nil {
		//TODO from docs:
		// you should only delete a folder locally if it is empty after syncing
		// all the changes.
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
	local := c.GetID(id)
	if local == nil {
		log.WithFields(log.Fields{
			"id":       id,
			"parentID": parentID,
			"name":     name,
			"delta":    "create",
		}).Info("Creating inode from delta.")
		c.InsertChild(parentID, delta)
		return nil
	}

	// was the item moved?
	if local.ParentID() != parentID || local.Name() != name {
		log.WithFields(log.Fields{
			"parent":    local.ParentID(),
			"name":      local.Name(),
			"newParent": parentID,
			"newName":   name,
			"id":        id,
			"delta":     "rename",
		}).Info("Applying server-side rename")
		parent := c.GetID(local.ParentID())
		newParent := c.GetID(parentID)
		if parent == nil || newParent == nil {
			log.WithFields(log.Fields{
				"parent":    local.ParentID(),
				"name":      local.Name(),
				"newParent": parentID,
				"newName":   name,
				"id":        id,
				"delta":     "rename",
			}).Error("Either original parent or new parent not found in cache!")
			return errors.New("Parent not in cache")
		}
		parent.Rename(context.Background(), local.Name(), newParent, name, 0)
		// do not return, there may be additional changes
	}

	// Finally, check if the content/metadata of the remote has changed.
	// "Interesting" changes must be synced back to our local state without
	// data loss or corruption. Currently the only thing the local filesystem
	// actually modifies remotely is the actual file data, so we simply accept
	// the remote metadata changes that do not deal with the file's content
	// changing.
	//
	// Do not sync if the file size is 0, as this is likely a file in the
	// progress of being uploaded (also, no need to sync empty files).
	if delta.ModTime() > local.ModTime() && delta.Size() > 0 {
		//TODO check if local has changes and rename the server copy if so
		log.WithFields(log.Fields{
			"id":    id,
			"name":  name,
			"delta": "overwrite",
		}).Info("Overwriting local item, no local changes to preserve.")
		// update modtime, hashes, purge local content
		c.DeleteContent(id)
		local.mutex.Lock()
		defer local.mutex.Unlock()
		local.ModTimeInternal = delta.ModTimeInternal
		local.FileInternal = delta.FileInternal
		local.hasChanges = false
		local.data = nil
		return nil
	}

	log.WithFields(log.Fields{
		"id":    id,
		"name":  name,
		"delta": "skip",
	}).Info("Skipping, no changes relative to local state.")
	return nil
}
