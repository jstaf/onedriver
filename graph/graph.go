// Package graph provides APIs to interact with Microsoft Graph
package graph

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/jstaf/onedriver/logger"
)

const graphURL = "https://graph.microsoft.com/v1.0"

// graphError is an internal struct used when decoding Graph's error messages
type graphError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Request performs an authenticated request to Microsoft Graph
func Request(resource string, auth *Auth, method string, content io.Reader) ([]byte, error) {
	if auth.AccessToken == "" {
		// a catch all condition to avoid wiping our auth by accident
		logger.Error("Auth was empty and we attempted to make a request with it!",
			"Guilty party was", logger.Caller(3), "called by", logger.Caller(4))
		return nil, errors.New("Cannot make a request with empty auth")
	}

	auth.Refresh()

	client := &http.Client{}
	request, _ := http.NewRequest(method, graphURL+resource, content)
	request.Header.Add("Authorization", "bearer "+auth.AccessToken)
	switch method { // request type-specific code here
	case "PATCH":
		request.Header.Add("If-Match", "*")
		request.Header.Add("Content-Type", "application/json")
	case "POST":
		request.Header.Add("Content-Type", "application/json")
	case "PUT":
		request.Header.Add("Content-Type", "text/plain")
	}

	response, err := client.Do(request)
	if err != nil {
		// the actual request failed
		return nil, err
	}
	defer response.Body.Close()
	body, _ := ioutil.ReadAll(response.Body)
	if response.StatusCode >= 400 {
		// something was wrong with the request
		var err graphError
		json.Unmarshal(body, &err)
		return nil, errors.New(err.Error.Code + ": " + err.Error.Message)
	}
	return body, nil
}

// Get is a convenience wrapper around Request
func Get(resource string, auth *Auth) ([]byte, error) {
	return Request(resource, auth, "GET", nil)
}

// Patch is a convenience wrapper around Request
func Patch(resource string, auth *Auth, content io.Reader) ([]byte, error) {
	return Request(resource, auth, "PATCH", content)
}

// Post is a convenience wrapper around Request
func Post(resource string, auth *Auth, content io.Reader) ([]byte, error) {
	return Request(resource, auth, "POST", content)
}

// Put is a convenience wrapper around Request
func Put(resource string, auth *Auth, content io.Reader) ([]byte, error) {
	return Request(resource, auth, "PUT", content)
}

// Delete performs an HTTP delete
func Delete(resource string, auth *Auth) error {
	_, err := Request(resource, auth, "DELETE", nil)
	return err
}

// ResourcePath translates an item's path to the proper path used by Graph
func ResourcePath(path string) string {
	if path == "/" {
		return "/me/drive/root"
	}
	return "/me/drive/root:" + path
}

// ChildrenPath returns the path to an item's children
func ChildrenPath(path string) string {
	if path == "/" {
		return ResourcePath(path) + "/children"
	}
	return ResourcePath(path) + ":/children"
}

// GetItem fetches a DriveItem by path
func GetItem(path string, auth *Auth) (*DriveItem, error) {
	body, err := Get(ResourcePath(path), auth)
	item := &DriveItem{
		mutex: &sync.RWMutex{},
	}
	if err != nil {
		return item, err
	}
	err = json.Unmarshal(body, item)
	return item, err
}
