package graph

import (
	"encoding/json"
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
		deltas := make(map[string]*DriveItem)
		for {
			incoming, cont, err := c.pollDeltas(c.auth)
			if err != nil {
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
		for _, item := range deltas {
			c.applyDelta(item)
		}

		// sleep till next sync interval
		log.Info("Sync complete!")
		time.Sleep(interval)
	}
}

type deltaResponse struct {
	NextLink  string       `json:"@odata.nextLink,omitempty"`
	DeltaLink string       `json:"@odata.deltaLink,omitempty"`
	Values    []*DriveItem `json:"value,omitempty"`
}

// Polls the delta endpoint and return deltas + whether or not to continue
// polling. Does not perform deduplication. Note that changes from the local
// client will actually appear as deltas from the server (there is no
// distinction between local and remote content from the server's perspective).
func (c *Cache) pollDeltas(auth *Auth) ([]*DriveItem, bool, error) {
	resp, err := Get(c.deltaLink, auth)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Could not fetch server deltas.")
		return make([]*DriveItem, 0), false, err
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
func (c *Cache) applyDelta(item *DriveItem) error {
	id := item.ID()
	log.WithFields(log.Fields{
		"id":   id,
		"name": item.Name(),
	}).Debug("Applying delta")

	// diagnose and act on what type of delta we're dealing with

	// do we have it at all?
	parentID := item.ParentID()
	if parent := c.GetID(parentID); parent == nil {
		// Nothing needs to be applied, item not in cache, so latest copy will
		// be pulled down next time it's accessed.
		log.WithFields(log.Fields{
			"id":       id,
			"parentID": parentID,
			"name":     item.Name(),
			"delta":    "skip",
		}).Trace("Skipping delta, item's parent not in cache.")
		return nil
	}

	// was it deleted?
	if item.Deleted != nil {
		//TODO from docs:
		// you should only delete a folder locally if it is empty after syncing
		// all the changes.
		log.WithFields(log.Fields{
			"id":    id,
			"name":  item.Name(),
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
			"name":     item.Name(),
			"delta":    "create",
		}).Info("Creating inode from delta.")
		c.InsertChild(parentID, item)
		return nil
	}

	// Finally, check if the content/metadata of the remote has changed.
	// "Interesting" changes must be synced back to our local state without
	// data loss or corruption. Currently the only thing the local filesystem
	// actually modifies remotely is the actual file data, so we simply accept
	// the remote metadata changes that do not deal with the file's content
	// changing.
	//TODO
	return nil
}
