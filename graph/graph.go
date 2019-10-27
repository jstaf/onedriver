// Package graph provides APIs to interact with Microsoft Graph
package graph

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/logger"
	log "github.com/sirupsen/logrus"
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
		log.WithFields(log.Fields{
			"caller":   logger.Caller(3),
			"calledBy": logger.Caller(4),
		}).Error("Auth was empty and we attempted to make a request with it!")
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

// ChildrenPathID returns the API resource path of an item's children
func ChildrenPathID(id string) string {
	return "/me/drive/items/" + id + "/children"
}

// DriveQuota is used to parse the User's current storage quotas from the API
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/quota
type DriveQuota struct {
	Deleted   uint64 `json:"deleted"`   // bytes in recycle bin
	FileCount uint64 `json:"fileCount"` // unavailable on personal accounts
	Remaining uint64 `json:"remaining"`
	State     string `json:"state"` // normal | nearing | critical | exceeded
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
}

// Drive has some general information about the user's OneDrive
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/drive
type Drive struct {
	ID        string     `json:"id"`
	DriveType string     `json:"driveType"` // personal or business
	Quota     DriveQuota `json:"quota,omitempty"`
}

// GetDrive is used to fetch the details of the user's OneDrive.
func GetDrive(auth *Auth) (Drive, error) {
	resp, err := Get("/me/drive", auth)
	drive := Drive{}
	if err != nil {
		return drive, err
	}
	return drive, json.Unmarshal(resp, &drive)
}

// GetItem fetches a DriveItem by ID. ID can also be "root" for the root item.
func GetItem(id string, auth *Auth) (*DriveItem, error) {
	path := "/me/drive/items/" + id
	if id == "root" {
		path = "/me/drive/root"
	}

	body, err := Get(path, auth)
	item := &DriveItem{}
	if err != nil {
		return item, err
	}
	err = json.Unmarshal(body, item)
	return item, err
}

// GetItemPath fetches a DriveItem by path. Only used in special cases, like for the
// root item.
func GetItemPath(path string, auth *Auth) (*DriveItem, error) {
	body, err := Get(ResourcePath(path), auth)
	item := &DriveItem{}
	if err != nil {
		return item, err
	}
	err = json.Unmarshal(body, item)
	return item, err
}

// GetItemContent retrieves an item's content from the Graph endpoint.
func GetItemContent(id string, auth *Auth) ([]byte, error) {
	return Get("/me/drive/items/"+id+"/content", auth)
}

// Remove removes a directory or file by ID
func Remove(id string, auth *Auth) error {
	return Delete("/me/drive/items/"+id, auth)
}

// Mkdir creates a directory on the server at the specified parent ID.
func Mkdir(name string, parentID string, auth *Auth) (*DriveItem, error) {
	// create a new folder on the server
	newFolderPost := APIItem{
		NameInternal: name,
		Folder:       &Folder{},
	}
	bytePayload, _ := json.Marshal(newFolderPost)
	resp, err := Post(ChildrenPathID(parentID), auth, bytes.NewReader(bytePayload))
	if err != nil {
		return nil, err
	}

	item := NewDriveItem(name, 0755|fuse.S_IFDIR, nil)
	err = json.Unmarshal(resp, &item)
	return item, err
}

// Rename moves and/or renames an item on the server. The itemName and parentID
// arguments correspond to the *new* basename or id of the parent.
func Rename(itemID string, itemName string, parentID string, auth *Auth) error {
	// start creating patch content for server
	// mutex does not need to be initialized since it is never used locally
	patchContent := APIItem{
		ConflictBehavior: "replace", // overwrite existing content at new location
		NameInternal:     itemName,
		Parent: &DriveItemParent{
			ID: parentID,
		},
	}

	// apply patch to server copy - note that we don't actually care about the
	// response content, only if it returns an error
	jsonPatch, _ := json.Marshal(patchContent)
	_, err := Patch("/me/drive/items/"+itemID, auth, bytes.NewReader(jsonPatch))
	if err != nil && strings.Contains(err.Error(), "resourceModified") {
		// Wait a second, then retry the request. The Onedrive servers sometimes
		// aren't quick enough here if the object has been recently created
		// (<1 second ago).
		time.Sleep(time.Second)
		_, err = Patch("/me/drive/items/"+itemID, auth, bytes.NewReader(jsonPatch))
	}
	return err
}
