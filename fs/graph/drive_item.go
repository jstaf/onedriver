package graph

import (
	"bytes"
	"encoding/json"
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
	ConflictBehavior string           `json:"@microsoft.graph.conflictBehavior,omitempty"`
}

// GetItem fetches a DriveItem by ID. ID can also be "root" for the root item.
func GetItem(id string, auth *Auth) (*DriveItem, error) {
	path := "/me/drive/items/" + id
	if id == "root" {
		path = "/me/drive/root"
	}

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

// GetItemPath fetches a DriveItem by path. Only used in special cases, like for the
// root item.
func GetItemPath(path string, auth *Auth) (*DriveItem, error) {
	body, err := Get(ResourcePath(path), auth)
	item := &DriveItem{}
	if err != nil {
		return item, err
	}
	err = json.Unmarshal(body, item)
	if err != nil && bytes.Contains(body, []byte("\"size\":-")) {
		// onedrive for business directories can sometimes have negative sizes,
		// ignore this error
		err = nil
	}
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
	newFolderPost := DriveItem{
		Name:   name,
		Folder: &Folder{},
	}
	bytePayload, _ := json.Marshal(newFolderPost)
	resp, err := Post(ChildrenPathID(parentID), auth, bytes.NewReader(bytePayload))
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
