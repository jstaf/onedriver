package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
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

// FuseFs is a memory-backed filesystem for Microsoft Graph
type FuseFs struct {
	pathfs.FileSystem
	Auth
	items    *ItemCache    // all DriveItems (read: files/folders) are cached
	reqCache *RequestCache // some requests are cached
}

// NewFS initializes a new Graph Filesystem to be used by go-fuse
func NewFS() *FuseFs {
	return &FuseFs{
		FileSystem: pathfs.NewDefaultFileSystem(),
		Auth:       Authenticate(),
		items:      &ItemCache{}, // lazily initialized on first use
		reqCache:   NewRequestCache(),
	}
}

// GetAttr returns a stat structure for the specified file
func (fs *FuseFs) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	name = "/" + name
	if ignore(name) {
		return nil, fuse.ENOENT
	}
	log.Printf("GetAttr(\"%s\")\n", name)
	item, err := fs.items.Get(name, fs.Auth)
	if err != nil {
		// this is where non-existent files are caught - called before any other
		// method when accessing a file
		log.Println(err)
		return nil, fuse.ENOENT
	}
	attr := fuse.Attr{}
	status := item.GetAttr(&attr)
	return &attr, status
}

// Chown currently does nothing - it is not a valid option, since fuse is single-user anyways
func (fs *FuseFs) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

// Chmod currently does nothing - no way to change mode yet.
func (fs *FuseFs) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

// OpenDir returns a list of directory entries
func (fs *FuseFs) OpenDir(name string, context *fuse.Context) (c []fuse.DirEntry, code fuse.Status) {
	name = "/" + name
	log.Printf("OpenDir(\"%s\")\n", name)

	parent, err := fs.items.Get(name, fs.Auth)
	if err != nil {
		log.Printf("Error getting item \"%s\": %s\n", name, err)
		return nil, fuse.EREMOTEIO
	}

	children, err := parent.GetChildren(fs.Auth)
	if err != nil {
		// not an item not found error (GetAttr() will always be called before
		// OpenDir()), something has happened to our connection
		log.Println("Error during OpenDir()", err)
		return nil, fuse.EREMOTEIO
	}

	for basename, child := range children {
		fmt.Println(basename)
		entry := fuse.DirEntry{
			Name: child.Name,
			Mode: child.Mode(),
		}
		c = append(c, entry)
	}

	return c, fuse.OK
}

type newFolderPost struct {
	Name   string   `json:"name"`
	Folder struct{} `json:"folder"`
}

// Mkdir creates a directory, mode is ignored
//TODO fix "File exists" case when folder is created, deleted, then created again
func (fs *FuseFs) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	name = "/" + name
	log.Printf("Mkdir(\"%s\")\n", name)

	bytePayload, _ := json.Marshal(newFolderPost{Name: filepath.Base(name)})
	resp, err := Post(ChildrenPath(filepath.Dir(name)), fs.Auth, bytes.NewReader(bytePayload))
	if err != nil {
		log.Println("Error during directory creation", err)
		log.Println(string(resp))
		return fuse.EREMOTEIO
	}
	return fuse.OK
}

// Rmdir removes a directory
func (fs *FuseFs) Rmdir(name string, context *fuse.Context) fuse.Status {
	name = "/" + name
	log.Printf("Rmdir(\"%s\")\n", name)
	err := Delete(ResourcePath(name), fs.Auth)
	if err != nil {
		log.Println("Error during delete:", err)
		return fuse.EREMOTEIO
	}
	fs.items.Delete(name)
	return fuse.OK
}

// Open populates a DriveItem's Data field with actual data
func (fs *FuseFs) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = "/" + name
	log.Printf("Open(\"%s\")\n", name)
	item, err := fs.items.Get(name, fs.Auth)
	if err != nil {
		// We know the file exists, GetAttr() has already been called
		log.Println("Error while getting item", err)
		return nil, fuse.EREMOTEIO
	}

	// check for if file has already been populated
	if item.Data == nil {
		// it is unpopulated, grab from api
		log.Println("Fetching remote content for", item.Name)
		err = item.FetchContent(fs.Auth)
		if err != nil {
			log.Printf("Failed to fetch content for '%s': %s\n", item.ID, err)
			return nil, fuse.EREMOTEIO
		}
	}
	return item, fuse.OK
}

// Create a new local file. The server doesn't have this yet.
func (fs *FuseFs) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = "/" + name
	log.Printf("Create(\"%s\")\n", name)

	// fetch details about the new item's parent (need the ID from the remote)
	parent, err := fs.items.Get(filepath.Dir(name), fs.Auth)
	if err != nil {
		log.Println("Error while fetching parent:", err)
		return nil, fuse.EREMOTEIO
	}

	item := NewDriveItem(filepath.Base(name), mode, parent)
	fs.items.Insert(name, fs.Auth, item)
	return item, fuse.OK
}
