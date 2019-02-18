package graph

import (
	"errors"
	"log"
	"path/filepath"
	"time"
)

// Cache is a generic cache interface intended to be overridden
type Cache interface {
	Delete(string)                         // wipes a cache item
	Get(string, Auth) (interface{}, error) // returns the item or an error
}

type expiringRequest struct {
	Response []byte
	Time     int64
}

// RequestCache is a map of past responses that we can check against.
// Keys of stored values must be a valid URI.
type RequestCache struct {
	Cache
	interval int64 // items in the cache expire after this interval
	cache    map[string]expiringRequest
}

// NewRequestCache produces a new request cache
func NewRequestCache() *RequestCache {
	return &RequestCache{
		interval: 30,
		cache:    make(map[string]expiringRequest),
	}
}

// Delete a request from the cache
func (c *RequestCache) Delete(key string) {
	delete(c.cache, key)
}

// Get performs a HTTP get request - if it's been performed in the last 10s it
// will just use the last response. Used to avoid swamping the API with useless
// requests and adding tons of latency.
func (c *RequestCache) Get(key string, auth Auth) ([]byte, error) {
	last, exists := c.cache[key]
	if exists && time.Now().Unix()-last.Time < c.interval {
		// we have a response that's less than 30 seconds old that's cached!
		return last.Response, nil
	}

	// no recent requestCache, fetch
	// note: not recursive, calls a standard Get to the Graph endpoint
	body, err := Get(key, auth)
	if err != nil {
		if exists {
			// send the out-of-date requestCache back, it's all we got
			return last.Response, nil
		}
		return nil, err
	}
	c.cache[key] = expiringRequest{
		Response: body,
		Time:     time.Now().Unix(),
	}
	return body, nil
}

// ItemCache caches DriveItems for a filesystem. This cache never expires so
// that local changes can persist.
type ItemCache struct {
	Cache
	root *DriveItem // will be a nil pointer on start, lazily initialized
}

// Get fetches a given DriveItem in the cache, if any items along the way are
// not found, they are fetched.
func (c *ItemCache) Get(key string, auth Auth) (*DriveItem, error) {
	// lazily initialize root of filesystem
	if c.root == nil {
		root, err := GetItem("/", auth)
		if err != nil {
			log.Fatal("Could not fetch root item of filesystem!:", err)
		}
		c.root = root
	}
	last := c.root

	split := filepath.SplitList(key)[1:] // omit leading "/"
	for i := 0; i < len(split); i++ {
		item, exists := last.Children[split[i]]
		if !exists {
			if auth.AccessToken == "" {
				return last, errors.New("Auth was empty and \"/" +
					last.Path() + "/" + split[i] +
					"\" was not in cache. Could not fetch item as a result.")
			}

			// we have an auth token and can try to fetch an item's children
			children, err := item.GetChildren(auth)
			if err != nil {
				return last, err
			}
			last = children[split[i]]
		} else {
			last = item
		}
	}
	return last, nil
}

// Delete an item from the cache
func (c *ItemCache) Delete(key string) {
	// Uses empty auth, since we actually don't want to waste time fetchin items
	// that are only being fetched so they can be deleted.
	parent, err := c.Get(filepath.Dir(key), Auth{})
	if err != nil {
		delete(parent.Children, filepath.Base(key))
	}
}

// Insert lets us manually insert an item to the cache (like if it was created
// locally). Overwrites a cached item if present.
func (c *ItemCache) Insert(resource string, auth Auth, item *DriveItem) error {
	parent, err := c.Get(filepath.Dir(resource), auth)
	if err != nil {
		return err
	}
	parent.Children[filepath.Base(resource)] = item
	return nil
}
