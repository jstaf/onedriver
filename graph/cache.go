package graph

import (
	"time"
)

type expiringRequest struct {
	Response []byte
	Time     int64
}

// cache is a map of past responses that we can check against
var cache = make(map[string]expiringRequest)

// CachedGet performs a get request - if it's been performed in the last 10s it
// will just use the last response. Used to avoid swamping the API with useless
// requests and adding tons of latency.
func CachedGet(resource string, auth Auth) ([]byte, error) {
	last, exists := cache[resource]
	if exists && time.Now().Unix()-last.Time < 10 {
		// we have a response that's less than 10 seconds old that's cached!
		return last.Response, nil
	}

	// no recent cache, fetch
	body, err := Get(resource, auth)
	if err != nil {
		if exists {
			// send the out-of-date cache back, it's all we got
			return last.Response, nil
		}
		return nil, err
	}
	cache[resource] = expiringRequest{
		Response: body,
		Time:     time.Now().Unix(),
	}
	return body, nil
}

// CacheClear deletes a file from the cache to force a refresh from the server
func CacheClear(resource string) {
	delete(cache, resource)
}
