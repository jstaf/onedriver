package graph

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/jstaf/onedriver/logger"
)

// these files will never exist, and we should ignore them
func ignore(path string) bool {
	ignoredFiles := []string{
		"/BDMV",
		"/.Trash",
		"/.Trash-1000",
		"/.xdg-volume-info",
		"/autorun.inf",
		"/.localized",
		"/.DS_Store",
		"/._.",
		"/.hidden",
	}
	for _, ignore := range ignoredFiles {
		if path == ignore {
			return true
		}
	}
	return false
}

func leadingSlash(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// FuseFs is a memory-backed filesystem for Microsoft Graph
type FuseFs struct {
	pathfs.FileSystem
	Auth
	items *ItemCache
}

// NewFS initializes a new Graph Filesystem to be used by go-fuse.
// Each method is executed concurrently as a goroutine.
func NewFS() *FuseFs {
	return &FuseFs{
		FileSystem: pathfs.NewDefaultFileSystem(),
		Auth:       Authenticate(),
		items:      &ItemCache{}, // lazily initialized on first use
	}
}

// GetAttr returns a stat structure for the specified file
func (fs *FuseFs) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	name = leadingSlash(name)
	if ignore(name) {
		return nil, fuse.ENOENT
	}
	logger.Trace(name)

	item, err := fs.items.Get(name, fs.Auth)
	if err != nil {
		// this is where non-existent files are caught - called before any other
		// method when accessing a file
		return nil, fuse.ENOENT
	}
	attr := fuse.Attr{}
	status := item.GetAttr(&attr)

	return &attr, status
}

// Rename is used by mv operations (move, rename)
func (fs *FuseFs) Rename(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	oldName, newName = leadingSlash(oldName), leadingSlash(newName)
	logger.Trace(oldName, "->", newName)

	item, _ := fs.items.Get(oldName, fs.Auth)
	item.ensureID(fs.Auth) //TODO can we get rid of this?
	if item.ID == "" {
		// uploads will fail without an id
		if item.IsDir() {
			logger.Error("ID of folder to move cannot be empty")
			return fuse.EBADF
		}

		if item.Upload(fs.Auth) != nil || item.ID == "" {
			logger.Error("ID of item to move cannot be empty " +
				"and we failed to obtain an ID.")
			return fuse.EBADF
		}
	}

	patchContent := DriveItem{} // totally empty to avoid sending extra data
	if newDir := filepath.Dir(newName); filepath.Dir(oldName) != newDir {
		// we are moving the item
		newParent, err := fs.items.Get(newDir, fs.Auth)
		if err != nil {
			logger.Errorf("Failed to fetch \"%s\": %s\n", newDir, err)
			return fuse.EREMOTEIO
		}
		if newParent.ID == "" {
			logger.Error("ID of destination folder cannot be empty!")
			return fuse.EBADF
		}
		patchContent.Parent = &DriveItemParent{ID: newParent.ID}
	}

	if newBase := filepath.Base(newName); filepath.Base(oldName) != newBase {
		// we are renaming the item
		patchContent.Name = newBase
		item.Name = newBase
	}

	jsonPatch, _ := json.Marshal(patchContent)
	// don't actually care about the response content
	_, err := Patch("/me/drive/items/"+item.ID, fs.Auth, bytes.NewReader(jsonPatch))
	if err != nil {
		logger.Error(err)
		item.Name = filepath.Base(oldName) // unrename things locally
		return fuse.EREMOTEIO
	}

	// rename local copy
	fs.items.Move(oldName, newName, fs.Auth)

	return fuse.OK
}

// Chown currently does nothing - it is not a valid option, since fuse is single-user anyways
func (fs *FuseFs) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

// Chmod changes mode purely for convenience/compatibility - it has no effect on
// server contents (onedrive has no notion of permissions).
func (fs *FuseFs) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	name = leadingSlash(name)
	item, _ := fs.items.Get(name, fs.Auth)
	return item.Chmod(mode)
}

// OpenDir returns a list of directory entries
func (fs *FuseFs) OpenDir(name string, context *fuse.Context) (c []fuse.DirEntry, code fuse.Status) {
	name = leadingSlash(name)
	logger.Trace(name)

	parent, err := fs.items.Get(name, fs.Auth)
	if err != nil {
		logger.Errorf("Error getting item \"%s\": %s\n", name, err)
		return nil, fuse.EREMOTEIO
	}

	children, err := parent.GetChildren(fs.Auth)
	if err != nil {
		// not an item not found error (GetAttr() will always be called before
		// OpenDir()), something has happened to our connection
		logger.Errorf("Error during OpenDir(\"%s\"): %s\n", name, err)
		return nil, fuse.EREMOTEIO
	}

	for _, child := range children {
		entry := fuse.DirEntry{
			Name: child.Name,
			Mode: child.Mode(),
		}
		c = append(c, entry)
	}

	return c, fuse.OK
}

// Mkdir creates a directory, mode is ignored
func (fs *FuseFs) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	name = leadingSlash(name)
	logger.Trace(name)

	// create a new folder on the server
	newFolderPost := DriveItem{
		Name:   filepath.Base(name),
		Folder: &Folder{},
	}
	bytePayload, _ := json.Marshal(newFolderPost)
	resp, err := Post(ChildrenPath(filepath.Dir(name)), fs.Auth, bytes.NewReader(bytePayload))
	if err != nil {
		logger.Error("Error during directory creation:", err)
		return fuse.EREMOTEIO
	}

	// create the new folder locally
	_, code := fs.Create(name, 0, mode|fuse.S_IFDIR, context)
	if code != fuse.OK {
		return code
	}

	// now unmarshal the response into the new folder so that it has an ID
	// (otherwise things involving this folder will fail later)
	item, _ := fs.items.Get(name, fs.Auth)
	json.Unmarshal(resp, item)

	return fuse.OK
}

// Rmdir removes a directory
func (fs *FuseFs) Rmdir(name string, context *fuse.Context) fuse.Status {
	name = leadingSlash(name)
	logger.Trace(name)

	err := Delete(ResourcePath(name), fs.Auth)
	if err != nil {
		logger.Error("Error during delete:", err)
		return fuse.EREMOTEIO
	}

	fs.items.Delete(name)

	return fuse.OK
}

// Open populates a DriveItem's Data field with actual data
func (fs *FuseFs) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = leadingSlash(name)
	logger.Trace(name)

	item, err := fs.items.Get(name, fs.Auth)
	if err != nil {
		// We know the file exists, GetAttr() has already been called
		logger.Error("Error while getting item", err)
		return nil, fuse.EREMOTEIO
	}

	// check for if file has already been populated
	if item.data == nil {
		// it is unpopulated, grab from api
		logger.Info("Fetching remote content for", item.Name)
		err = item.FetchContent(fs.Auth)
		if err != nil {
			logger.Errorf("Failed to fetch content for '%s': %s\n", item.ID, err)
			return nil, fuse.EREMOTEIO
		}
	}
	return item, fuse.OK
}

// Create a new local file. The server doesn't have this yet.
func (fs *FuseFs) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = leadingSlash(name)
	logger.Trace(name)

	// fetch details about the new item's parent (need the ID from the remote)
	parent, err := fs.items.Get(filepath.Dir(name), fs.Auth)
	if err != nil {
		logger.Error("Error while fetching parent:", err)
		return nil, fuse.EREMOTEIO
	}

	item := NewDriveItem(filepath.Base(name), mode, parent)
	fs.items.Insert(name, fs.Auth, item)

	return item, fuse.OK
}

// Unlink deletes a file
func (fs *FuseFs) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	name = leadingSlash(name)
	logger.Trace(name)

	item, _ := fs.items.Get(name, fs.Auth)
	if item.ID != "" {
		err := Delete(ResourcePath(name), fs.Auth)
		if err != nil {
			logger.Error(err)
			return fuse.EREMOTEIO
		}
	}

	fs.items.Delete(name)

	return fuse.OK
}
