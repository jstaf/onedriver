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
		log.Debug("Syncing deltas from server.")
		for {
			//TODO should poll and dedup deltas here, then act on them in a
			// separate block
			cont, err := c.pollDeltas(c.auth)
			if err != nil {
				log.Error(err)
				break
			}
			if !cont {
				break
			}
		}
		log.Debug("Sync complete!")
		time.Sleep(interval)
	}
}

type deltaResponse struct {
	NextLink  string      `json:"@odata.nextLink,omitempty"`
	DeltaLink string      `json:"@odata.deltaLink,omitempty"`
	Values    []DriveItem `json:"value,omitempty"`
}

// Polls the delta endpoint and return whether or not to continue polling
func (c *Cache) pollDeltas(auth *Auth) (bool, error) {
	resp, err := Get(c.deltaLink, auth)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Could not fetch server deltas.")
		return false, err
	}

	page := deltaResponse{}
	json.Unmarshal(resp, &page)
	for _, item := range page.Values {
		//TODO should dedup deltas here, and use the last one received as
		// recommended by API documentation
		c.applyDelta(item)
	}

	// If the server does not provide a `@odata.nextLink` item, it means we've
	// reached the end of this polling cycle and should not continue until the
	// next poll interval.
	if page.NextLink != "" {
		c.deltaLink = strings.TrimPrefix(page.NextLink, graphURL)
		return true, nil
	}
	c.deltaLink = strings.TrimPrefix(page.DeltaLink, graphURL)
	return false, nil
}

// apply a server-side change to our local state
func (c *Cache) applyDelta(item DriveItem) error {
	log.WithFields(log.Fields{
		"id":   item.ID(),
		"name": item.Name(),
	}).Debug("Applying delta")

	// diagnose and act on what type of delta we're dealing with

	// do we have it at all?
	if parent := c.GetID(item.ParentID()); parent == nil {
		// Nothing needs to be applied, item not in cache, so latest copy will
		// be pulled down next time it's accessed.
		log.WithFields(log.Fields{
			"name":     item.Name(),
			"parentID": item.ParentID(),
			"delta":    "skip",
		}).Trace("Skipping delta, item's parent not in cache.")
		return nil
	}

	// was it deleted?
	if item.Deleted != nil {
		log.WithFields(log.Fields{
			"id":    item.ID(),
			"name":  item.Name(),
			"delta": "delete",
		}).Info("Applying server-side deletion of item")
		c.DeleteID(item.ID())
		return nil
	}

	//TODO stub
	return nil
}
