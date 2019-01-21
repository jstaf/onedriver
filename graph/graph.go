package graph

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

const graphURL = "https://graph.microsoft.com/v1.0"

// graphError is an internal struct used when decoding Graph's error messages
type graphError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// DriveItem represents a drive item's worth of info fetched from the Graph API
type DriveItem struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Size       int       `json:"size"`
	ModifyTime time.Time `json:"lastModifiedDatetime"` // a string timestamp
	Parent     struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	} `json:"parentReference"`
	Folder struct {
		ChildCount int `json:"childCount"`
	} `json:"folder,omitempty"`
	File struct {
		Hashes struct {
			Sha1Hash string `json:"sha1Hash"`
		} `json:"hashes"`
	} `json:"file,omitempty"`
}

// IsDir returns if it is a directory (true) or file (false).
func (d DriveItem) IsDir() bool {
	return d.File.Hashes.Sha1Hash == ""
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
	body, err := Get(resourcePath(path), auth)
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
	body, err := Get(path, auth)
	var children driveChildren
	if err != nil {
		return children.Children, err
	}
	json.Unmarshal(body, &children)
	return children.Children, nil
}
