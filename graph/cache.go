package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jstaf/onedriver/logger"
)

// Cache caches DriveItems for a filesystem. This cache never expires so
// that local changes can persist. Should be created using the NewCache()
// constructor.
type Cache struct {
	root      *DriveItem
	deltaLink string
}

// NewCache creates a new Cache
func NewCache(auth Auth) *Cache {
	cache := &Cache{}
	root, err := GetItem("/", auth)
	if err != nil {
		logger.Fatal("Could not fetch root item of filesystem!:", err)
	}
	root.auth = &auth
	cache.root = root

	// using token=latest because we don't care about existing items - they'll
	// be downloaded on-demand by the cache
	cache.deltaLink = "/me/drive/root/delta?token=latest"
	// deltaloop is started manually
	return cache
}

// Get fetches a given DriveItem in the cache, if any items along the way are
// not found, they are fetched.
func (c *Cache) Get(key string, auth Auth) (*DriveItem, error) {
	last := c.root

	// from the root directory, traverse the chain of items till we reach our
	// target key
	key = strings.ToLower(key)
	key = strings.TrimSuffix(key, "/")
	split := strings.Split(key, "/")[1:] // omit leading "/"
	for i := 0; i < len(split); i++ {
		last.mutex.RLock()
		item, exists := last.children[split[i]]
		last.mutex.RUnlock()
		if !exists {
			if auth.AccessToken == "" {
				return last, errors.New("Auth was empty and \"" +
					filepath.Join(last.Path(), split[i]) +
					"\" was not in cache. Could not fetch item as a result.")
			}

			// we have an auth token and can try to fetch an item's children
			children, err := last.GetChildren(auth)
			if err != nil {
				return last, err
			}
			last.mutex.RLock()
			item, exists = children[split[i]]
			last.mutex.RUnlock()
			if !exists {
				// this time, we know the key *really* doesn't exist
				return nil, errors.New(filepath.Join(last.Path(), split[i]) + " does not exist.")
			}
		}
		last = item
	}
	return last, nil
}

// Delete an item from the cache
func (c *Cache) Delete(key string) {
	key = strings.ToLower(key)
	// Uses empty auth, since we actually don't want to waste time fetching
	// items that are only being fetched so they can be deleted.
	parent, err := c.Get(filepath.Dir(key), Auth{})
	if err == nil {
		parent.mutex.Lock()
		delete(parent.children, filepath.Base(key))
		parent.mutex.Unlock()
	}
}

// Insert lets us manually insert an item to the cache (like if it was created
// locally). Overwrites a cached item if present.
func (c *Cache) Insert(key string, auth Auth, item *DriveItem) error {
	key = strings.ToLower(key)
	parent, err := c.Get(filepath.Dir(key), auth)
	if err != nil {
		return err
	}
	item.setParent(parent)
	parent.mutex.Lock()
	parent.children[filepath.Base(key)] = item
	parent.mutex.Unlock()
	return nil
}

// Move an item to a new position
func (c *Cache) Move(oldPath string, newPath string, auth Auth) error {
	item, err := c.Get(oldPath, auth)
	if err != nil {
		return err
	}
	// insert first, so data is not lost in the event the insert fails
	if err = c.Insert(newPath, auth, item); err != nil {
		return err
	}
	c.Delete(oldPath)
	return nil
}

// deltaLoop should be called as a goroutine
func (c *Cache) deltaLoop(auth *Auth) {
	logger.Trace("Starting delta goroutine...")
	for { // eva
		// get deltas
		logger.Trace("Beginning sync...")
		for {
			cont, err := c.pollDeltas(auth)
			if err != nil {
				logger.Error(err)
				break
			}
			if !cont {
				break
			}
		}

		// go to sleep until next poll interval
		time.Sleep(30 * time.Second)
	}
}

type deltaResponse struct {
	NextLink  string      `json:"@odata.nextLink,omitempty"`
	DeltaLink string      `json:"@odata.deltaLink,omitempty"`
	Values    []DriveItem `json:"value,omitempty"`
}

// Polls the delta endpoint and return whether or not to continue polling
func (c *Cache) pollDeltas(auth *Auth) (bool, error) {
	logger.Trace("Polling deltas...")
	resp, err := Get(c.deltaLink, *auth)
	if err != nil {
		logger.Error("Could not fetch server deltas:", err)
		return false, err
	}

	//TODO while developing for the first little bit
	logger.Info("Delta response:")
	fmt.Println(string(resp))

	page := deltaResponse{}
	json.Unmarshal(resp, &page)
	for _, item := range page.Values {
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
	logger.Trace("Applying delta for", item.Name)
	//TODO stub
	return nil
}
