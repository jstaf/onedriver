package graph

import (
	"path/filepath"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	log "github.com/sirupsen/logrus"
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
	*Auth
	items *Cache
}

// NewFS initializes a new Graph Filesystem to be used by go-fuse.
// Each method is executed concurrently as a goroutine.
func NewFS(dbpath string) *FuseFs {
	auth := Authenticate()
	cache := NewCache(auth, dbpath)
	//go cache.deltaLoop(time.Second * 30)
	return &FuseFs{
		FileSystem: pathfs.NewDefaultFileSystem(),
		Auth:       auth,
		items:      cache,
	}
}

// GetAttr returns a stat structure for the specified file
func (fs *FuseFs) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	name = leadingSlash(name)
	if ignore(name) {
		return nil, fuse.ENOENT
	}

	item, err := fs.items.GetPath(name, fs.Auth)
	if err != nil || item == nil {
		// this is where non-existent files are caught - called before any other
		// method when accessing a file
		return nil, fuse.ENOENT
	}
	log.WithFields(log.Fields{"path": name}).Trace()

	attr := fuse.Attr{}
	status := item.GetAttr(&attr)

	return &attr, status
}

// Rename is used by mv operations (move, rename)
func (fs *FuseFs) Rename(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	oldName, newName = leadingSlash(oldName), leadingSlash(newName)
	log.WithFields(log.Fields{
		"path": oldName,
		"dest": newName,
	}).Debug()

	// grab item being renamed
	item, _ := fs.items.GetPath(oldName, fs.Auth)
	id, err := item.RemoteID(fs.Auth)
	if isLocalID(id) || err != nil {
		// uploads will fail without an id
		log.WithFields(log.Fields{
			"id":   id,
			"path": oldName,
			"err":  err,
		}).Error("ID of item to move cannot be local and we failed to obtain an ID.")
		return fuse.EBADF
	}

	// perform remote rename
	newParent, err := fs.items.GetPath(filepath.Dir(newName), fs.Auth)
	if err != nil {
		log.WithFields(log.Fields{
			"path": filepath.Dir(newName),
			"err":  err,
		}).Error("Failed to fetch new parent item.")
		return fuse.ENOENT
	}

	parentID := newParent.ID()
	if isLocalID(parentID) || err != nil {
		// should never be reached, but being extra safe here
		log.WithFields(log.Fields{
			"id":   parentID,
			"path": filepath.Dir(newName),
			"err":  err,
		}).Error("ID of destination folder cannot be local.")
		return fuse.EBADF
	}

	err = Rename(id, filepath.Base(newName), parentID, fs.Auth)
	if err != nil {
		log.WithFields(log.Fields{
			"id":       id,
			"parentID": parentID,
			"err":      err,
		}).Error("Failed to rename remote item.")
		return fuse.EREMOTEIO
	}

	// now rename local copy
	if err = fs.items.MovePath(oldName, newName, fs.Auth); err != nil {
		log.WithFields(log.Fields{
			"path": oldName,
			"dest": newName,
			"err":  err,
		}).Error("Failed to rename local item.")
		return fuse.EIO
	}
	return fuse.OK
}

// Chown currently does nothing - it is not a valid option, since fuse is single-user anyways
func (fs *FuseFs) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

// Chmod changes mode purely for convenience/compatibility - it has no effect on
// server contents (onedrive has no notion of permissions).
func (fs *FuseFs) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	name = leadingSlash(name)
	item, _ := fs.items.GetPath(name, fs.Auth)
	return item.Chmod(mode)
}

// OpenDir returns a list of directory entries
func (fs *FuseFs) OpenDir(name string, context *fuse.Context) (c []fuse.DirEntry, code fuse.Status) {
	name = leadingSlash(name)
	log.WithFields(log.Fields{"path": name}).Debug()

	children, err := fs.items.GetChildrenPath(name, fs.Auth)
	if err != nil {
		// not an item not found error (GetAttr() will always be called before
		// OpenDir()), something has happened to our connection
		log.WithFields(log.Fields{
			"path": name,
			"err":  err,
		}).Error("Error during OpenDir()")
		return nil, fuse.EREMOTEIO
	}

	for _, child := range children {
		entry := fuse.DirEntry{
			Name: child.Name(),
			Mode: child.Mode(),
		}
		c = append(c, entry)
	}

	return c, fuse.OK
}

// Mkdir creates a directory, mode is ignored
func (fs *FuseFs) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	name = leadingSlash(name)
	log.WithFields(log.Fields{"path": name}).Debug()

	// create a new folder on the server
	item, err := Mkdir(name, fs.Auth)
	if err != nil {
		log.WithFields(log.Fields{
			"path": name,
			"err":  err,
		}).Error("Error during directory creation:")
		return fuse.EREMOTEIO
	}

	// create the new folder locally
	created, code := fs.Create(name, 0, mode|fuse.S_IFDIR, context)
	if code != fuse.OK {
		return code
	}

	// Move the directory to be stored under the non-local ID.
	//TODO: eliminate the need for renames after Create()
	if fs.items.MoveID(created.(*DriveItem).ID(), item.ID()) != nil {
		return fuse.EIO
	}
	return fuse.OK
}

// Rmdir removes a directory
func (fs *FuseFs) Rmdir(name string, context *fuse.Context) fuse.Status {
	name = leadingSlash(name)
	log.WithFields(log.Fields{"path": name}).Debug()

	if err := Remove(name, fs.Auth); err != nil {
		log.WithFields(log.Fields{
			"path": name,
			"err":  err,
		}).Error("Error during delete")
		return fuse.EREMOTEIO
	}

	fs.items.DeletePath(name)
	return fuse.OK
}

// Open fetches a DriveItem's content and initializes the .Data field with
// actual data from the server. Data is loaded into memory on Open, and
// persisted to disk on Flush.
func (fs *FuseFs) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = leadingSlash(name)
	log.WithFields(log.Fields{"path": name}).Debug()

	item, err := fs.items.GetPath(name, fs.Auth)
	if err != nil {
		// We know the file exists, GetAttr() has already been called
		log.WithFields(log.Fields{
			"path": name,
			"err":  err,
		}).Error("Error fetching item from cache")
		return nil, fuse.EREMOTEIO
	}

	if item.HasContent() {
		// somehow we already have data
		return item, fuse.OK
	}

	// try grabbing from disk
	id := item.ID()
	if content := fs.items.GetContent(id); content != nil {
		// TODO should verify from cache using hash from server
		log.WithFields(log.Fields{
			"path": name,
			"id":   id,
		}).Info("Found content in cache.")

		item.mutex.Lock()
		defer item.mutex.Unlock()
		// this check is here in case the onedrive file sizes are WRONG.
		// (it happens)
		item.SizeInternal = uint64(len(content))
		item.data = &content
		item.File = nodefs.NewDefaultFile()
		return item, fuse.OK
	}

	// didn't have it on disk, now try api
	log.WithFields(log.Fields{
		"path": name,
	}).Info("Fetching remote content for item from API")

	id, err = item.RemoteID(fs.Auth)
	if err != nil || id == "" {
		log.WithFields(log.Fields{
			"id":   id,
			"name": item.Name(),
			"err":  err,
		}).Error("Could not obtain remote ID.")
		return nil, fuse.EREMOTEIO
	}

	body, err := GetItemContent(id, fs.Auth)
	if err != nil {
		log.WithFields(log.Fields{
			"err":  err,
			"id":   id,
			"path": name,
		}).Error("Failed to fetch remote content")
		return nil, fuse.EREMOTEIO
	}

	item.mutex.Lock()
	defer item.mutex.Unlock()
	// this check is here in case the onedrive file sizes are WRONG.
	// (it happens)
	item.SizeInternal = uint64(len(body))
	item.data = &body
	item.File = nodefs.NewDefaultFile()
	return item, fuse.OK
}

// Create a new local file. The server doesn't have this yet.
func (fs *FuseFs) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = leadingSlash(name)
	log.WithFields(log.Fields{"path": name}).Debug()

	// fetch details about the new item's parent (need the ID from the remote)
	parent, err := fs.items.GetPath(filepath.Dir(name), fs.Auth)
	if err != nil {
		log.WithFields(log.Fields{
			"path": name,
			"err":  err,
		}).Error("Error while fetching parent.")
		return nil, fuse.EREMOTEIO
	}

	item := NewDriveItem(filepath.Base(name), mode, parent)
	err = fs.items.InsertPath(name, fs.Auth, item)
	if err != nil {
		log.WithFields(log.Fields{
			"err":  err,
			"path": name,
			"id":   item.ID(),
		}).Error("Failed to insert item into cache.")
	}

	return item, fuse.OK
}

// Unlink deletes a file
func (fs *FuseFs) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	name = leadingSlash(name)
	log.WithFields(log.Fields{"path": name}).Debug()

	item, err := fs.items.GetPath(name, fs.Auth)
	// allow safely calling Unlink on items that don't actually exist
	if err != nil && strings.Contains(err.Error(), "does not exist") {
		return fuse.ENOENT
	}

	// if no ID, the item is local-only, and does not need to be deleted on the
	// server
	if !isLocalID(item.ID()) {
		if err = Remove(name, fs.Auth); err != nil {
			log.WithFields(log.Fields{
				"err":  err,
				"path": name,
			}).Error("Failed to delete item on server. Aborting op.")
			return fuse.EREMOTEIO
		}
	}

	fs.items.DeletePath(name)

	return fuse.OK
}
