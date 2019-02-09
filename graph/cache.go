package graph

import (
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

// ItemCache caches DriveItems for a filesystem
type ItemCache struct {
	Cache
	cache map[string]*DriveItem // must be pointers or writes are not honored
}

// NewItemCache initializes an returns a pointer to a new ItemCache
func NewItemCache() *ItemCache {
	return &ItemCache{cache: make(map[string]*DriveItem)}
}

// Delete an item from the cache
func (c *ItemCache) Delete(key string) {
	delete(c.cache, key)
}

// Get fetches an item from the cache. This cache never expires (so local
// changes can persist).
func (c *ItemCache) Get(resource string, auth Auth) (*DriveItem, error) {
	last, exists := c.cache[resource]
	if exists {
		return last, nil
	}
	item, err := GetItem(resource, auth)
	if err == nil {
		c.cache[resource] = item
	}
	return item, err
}

// Insert lets us manually insert an item to the cache (like if it was created
// locally). Overwrites a cached item if present.
func (c *ItemCache) Insert(resource string, item *DriveItem) {
	c.cache[resource] = item
}
