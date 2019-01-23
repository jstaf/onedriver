// Package graph provides APIs to interact with Microsoft Graph
package graph

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
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
func Request(resource string, auth Auth, method string, content io.Reader) ([]byte, error) {
	auth.Refresh()
	client := &http.Client{}
	request, _ := http.NewRequest("GET", graphURL+resource, content)
	request.Header.Add("Authorization", "bearer "+auth.AccessToken)
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
func Get(resource string, auth Auth) ([]byte, error) {
	return Request(resource, auth, "GET", nil)
}

// Post is a convenience wrapper around Request
func Post(resource string, auth Auth, content io.Reader) ([]byte, error) {
	return Request(resource, auth, "Post", content)
}

// Translate's an item's path to the proper path used by Graph
func resourcePath(path string) string {
	if path == "/" {
		return "/me/drive/root"
	}
	return "/me/drive/root:" + path
}

// GetItem fetches a DriveItem by path
func GetItem(path string, auth Auth) (DriveItem, error) {
	body, err := CachedGet(resourcePath(path), auth)
	var item DriveItem
	if err != nil {
		return item, err
	}
	json.Unmarshal(body, &item)
	return item, nil
}

// only used for parsing
type driveChildren struct {
	Children []DriveItem `json:"value"`
}

// GetChildren fetches all DriveItems that are children of resource at path
func GetChildren(path string, auth Auth) ([]DriveItem, error) {
	if path == "/" {
		path = resourcePath(path) + "/children"
	} else {
		path = resourcePath(path) + ":/children"
	}
	body, err := CachedGet(path, auth)
	var children driveChildren
	if err != nil {
		return children.Children, err
	}
	json.Unmarshal(body, &children)
	return children.Children, nil
}
