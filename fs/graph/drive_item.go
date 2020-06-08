package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// DriveTypePersonal and friends represent the possible different values for a
// drive's type when fetched from the API.
const (
	DriveTypePersonal   = "personal"
	DriveTypeBusiness   = "business"
	DriveTypeSharepoint = "documentLibrary"
)

// DriveItemParent describes a DriveItem's parent in the Graph API
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/itemreference
type DriveItemParent struct {
	//TODO Path is technically available, but we shouldn't use it
	Path      string `json:"path,omitempty"`
	ID        string `json:"id,omitempty"`
	DriveID   string `json:"driveId,omitempty"`
	DriveType string `json:"driveType,omitempty"` // personal | business | documentLibrary
}

// Folder is used for parsing only
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/folder
type Folder struct {
	ChildCount uint32 `json:"childCount,omitempty"`
}

// Hashes are integrity hashes used to determine if file content has changed.
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/hashes
type Hashes struct {
	SHA1Hash     string `json:"sha1Hash,omitempty"`
	QuickXorHash string `json:"quickXorHash,omitempty"`
}

// File is used for checking for changes in local files (relative to the server).
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/file
type File struct {
	Hashes Hashes `json:"hashes,omitempty"`
}

// Deleted is used for detecting when items get deleted on the server
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/deleted
type Deleted struct {
	State string `json:"state,omitempty"`
}

// RemoteItem is a facet that appears for shared items only.
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/api/drive_sharedwithme
type RemoteItem struct {
	ID     string           `json:"id,omitempty"`
	Parent *DriveItemParent `json:"parentReference,omitempty"`
}

// DriveItem contains the data fields from the Graph API
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/driveitem
type DriveItem struct {
	ID               string           `json:"id,omitempty"`
	Name             string           `json:"name,omitempty"`
	Size             uint64           `json:"size,omitempty"`
	ModTime          *time.Time       `json:"lastModifiedDatetime,omitempty"`
	Parent           *DriveItemParent `json:"parentReference,omitempty"`
	Folder           *Folder          `json:"folder,omitempty"`
	File             *File            `json:"file,omitempty"`
	Deleted          *Deleted         `json:"deleted,omitempty"`
	RemoteItem       *RemoteItem      `json:"remoteItem,omitempty"`
	ConflictBehavior string           `json:"@microsoft.graph.conflictBehavior,omitempty"`
	ETag             string           `json:"eTag,omitempty"`
}

// IsDir returns if the DriveItem represents a directory or not
func (d *DriveItem) IsDir() bool {
	return d.Folder != nil
}

// IsShared returns whether an item is shared from another drive
func (d *DriveItem) IsShared() bool {
	return d.RemoteItem != nil
}

// ModTimeUnix returns the modification time as a unix uint64 time
func (d *DriveItem) ModTimeUnix() uint64 {
	return uint64(d.ModTime.Unix())
}

// getItem is the internal method used to lookup items
func getItem(path string, auth *Auth) (*DriveItem, error) {
	body, err := Get(path, auth)
	if err != nil {
		return nil, err
	}
	item := &DriveItem{}
	err = json.Unmarshal(body, item)
	if err != nil && bytes.Contains(body, []byte("\"size\":-")) {
		// onedrive for business directories can sometimes have negative sizes,
		// ignore this error
		err = nil
	}
	return item, err
}

// GetItem fetches a DriveItem by ID. ID can also be "root" for the root item.
func GetItem(id string, auth *Auth) (*DriveItem, error) {
	return getItem(IDPath(id), auth)
}

// GetItemChild fetches the named child of an item.
func GetItemChild(id string, name string, auth *Auth) (*DriveItem, error) {
	return getItem(
		fmt.Sprintf("%s:/%s", IDPath(id), url.PathEscape(name)),
		auth,
	)
}

// GetItemPath fetches a DriveItem by path. Only used in special cases, like for the
// root item.
func GetItemPath(path string, auth *Auth) (*DriveItem, error) {
	return getItem(ResourcePath(path), auth)
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
	newFolderPost := DriveItem{
		Name:   name,
		Folder: &Folder{},
	}
	bytePayload, _ := json.Marshal(newFolderPost)
	resp, err := Post(childrenPathID(parentID), auth, bytes.NewReader(bytePayload))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(resp, &newFolderPost)
	return &newFolderPost, err
}

// Rename moves and/or renames an item on the server. The itemName and parentID
// arguments correspond to the *new* basename or id of the parent.
func Rename(itemID string, itemName string, parentID string, auth *Auth) error {
	// start creating patch content for server
	// mutex does not need to be initialized since it is never used locally
	patchContent := DriveItem{
		ConflictBehavior: "replace", // overwrite existing content at new location
		Name:             itemName,
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

// only used for parsing
type driveChildren struct {
	Children []*DriveItem `json:"value"`
	NextLink string       `json:"@odata.nextLink"`
}

// this is the internal method that actually fetches an item's children
func getItemChildren(pollURL string, auth *Auth) ([]*DriveItem, error) {
	fetched := make([]*DriveItem, 0)
	for pollURL != "" {
		body, err := Get(pollURL, auth)
		if err != nil {
			return fetched, err
		}
		var pollResult driveChildren
		json.Unmarshal(body, &pollResult)

		// there can be multiple pages of 200 items each (default).
		// continue to next interation if we have an @odata.nextLink value
		fetched = append(fetched, pollResult.Children...)
		pollURL = strings.TrimPrefix(pollResult.NextLink, GraphURL)
	}
	return fetched, nil
}

// GetItemChildren fetches all children of an item denoted by ID.
func GetItemChildren(id string, auth *Auth) ([]*DriveItem, error) {
	return getItemChildren(childrenPathID(id), auth)
}

// GetItemChildrenPath fetches all children of an item denoted by path.
func GetItemChildrenPath(path string, auth *Auth) ([]*DriveItem, error) {
	return getItemChildren(childrenPath(path), auth)
}
