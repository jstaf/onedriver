package graph

import (
	"time"
)

type expiringRequest struct {
	Response []byte
	Time     int64
}

// requestCache is a map of past responses that we can check against
var requestCache = make(map[string]expiringRequest)
var itemCache = make(map[string]DriveItem)

// CacheGet performs a get request - if it's been performed in the last 10s it
// will just use the last response. Used to avoid swamping the API with useless
// requests and adding tons of latency.
func CacheGet(resource string, auth Auth) ([]byte, error) {
	last, exists := requestCache[resource]
	if exists && time.Now().Unix()-last.Time < 30 {
		// we have a response that's less than 30 seconds old that's cached!
		return last.Response, nil
	}

	// no recent requestCache, fetch
	body, err := Get(resource, auth)
	if err != nil {
		if exists {
			// send the out-of-date requestCache back, it's all we got
			return last.Response, nil
		}
		return nil, err
	}
	requestCache[resource] = expiringRequest{
		Response: body,
		Time:     time.Now().Unix(),
	}
	return body, nil
}

// CacheGetItem fetches an item from the cache. This cache never expires. This
// allows local changes to persist.
func CacheGetItem(resource string, auth Auth) (DriveItem, error) {
	last, exists := itemCache[resource]
	if exists {
		return last, nil
	}
	item, err := GetItem(resource, auth)
	if err == nil {
		itemCache[resource] = item
	}
	return item, err
}

// CacheClear deletes a file from the requestCache to force a refresh from the server
func CacheClear(resource string) {
	delete(requestCache, resource)
}
