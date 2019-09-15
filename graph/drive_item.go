package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	mu "github.com/sasha-s/go-deadlock"
	log "github.com/sirupsen/logrus"
)

// DriveItemParent describes a DriveItem's parent in the Graph API (just another
// DriveItem's ID and its path)
type DriveItemParent struct {
	//TODO Path is technically available, but we shouldn't use it
	Path string `json:"path,omitempty"`
	ID   string `json:"id,omitempty"`
}

// Folder is used for parsing only
type Folder struct {
	ChildCount uint32 `json:"childCount,omitempty"`
}

// File is used for parsing only
type File struct {
	MimeType string `json:"mimeType,omitempty"`
}

// Deleted is used for detecting when items get deleted on the server
type Deleted struct {
	State string `json:"state,omitempty"`
}

// APIItem contains the data fields from the Graph API
type APIItem struct {
	IDInternal       string           `json:"id,omitempty"`
	NameInternal     string           `json:"name,omitempty"`
	SizeInternal     uint64           `json:"size,omitempty"`
	ModTimeInternal  *time.Time       `json:"lastModifiedDatetime,omitempty"`
	Parent           *DriveItemParent `json:"parentReference,omitempty"`
	Folder           *Folder          `json:"folder,omitempty"`
	FileInternal     *File            `json:"file,omitempty"`
	Deleted          *Deleted         `json:"deleted,omitempty"`
	ConflictBehavior string           `json:"@microsoft.graph.conflictBehavior,omitempty"`
}

// DriveItem represents a file or folder fetched from the Graph API. All struct
// fields are pointers so as to avoid including them when marshaling to JSON
// if not present. Fields named "xxxxxInternal" should never be accessed, they
// are there for JSON umarshaling/marshaling only. (They are not safe to access
// concurrently.) This struct's methods are thread-safe and can be called
// concurrently. Reads/writes are done directly to DriveItems instead of
// implementing something like the fs.FileHandle to minimize the complexity of
// operations like Flush.
type DriveItem struct {
	// fs fields
	fs.Inode `json:"-"`
	APIItem
	cache         *Cache
	mutex         mu.RWMutex     // used to be a pointer, but fs.Inode also embeds a mutex :(
	children      []string       // a slice of ids, nil when uninitialized
	uploadSession *UploadSession // current upload session, or nil
	data          *[]byte        // empty by default
	hasChanges    bool           // used to trigger an upload on flush
	subdir        uint32         // used purely by NLink()
	mode          uint32         // do not set manually
}

// NewDriveItem initializes a new DriveItem
func NewDriveItem(name string, mode uint32, parent *DriveItem) *DriveItem {
	itemParent := &DriveItemParent{ID: "", Path: ""}
	if parent != nil {
		itemParent.ID = parent.ID()
		itemParent.Path = parent.Path()
	}

	var empty []byte
	currentTime := time.Now()
	return &DriveItem{
		APIItem: APIItem{
			IDInternal:      localID(),
			NameInternal:    name,
			Parent:          itemParent,
			ModTimeInternal: &currentTime,
		},
		children: make([]string, 0),
		data:     &empty,
		mode:     mode,
	}
}

// String is only used for debugging by go-fuse
func (d *DriveItem) String() string {
	return d.Name()
}

// Name is used to ensure thread-safe access to the NameInternal field.
func (d *DriveItem) Name() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.NameInternal
}

// SetName sets the name of the item in a thread-safe manner.
func (d *DriveItem) SetName(name string) {
	d.mutex.Lock()
	d.NameInternal = name
	d.mutex.Unlock()
}

var charset = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func randString(length int) string {
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = charset[rand.Intn(len(charset))]
	}
	return string(out)
}

func localID() string {
	return "local-" + randString(20)
}

func isLocalID(id string) bool {
	return strings.HasPrefix(id, "local-") || id == ""
}

// ID returns the internal ID of the item
func (d *DriveItem) ID() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.IDInternal
}

// ParentID returns the ID of this item's parent.
func (d *DriveItem) ParentID() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if d.APIItem.Parent == nil {
		return ""
	}
	return d.APIItem.Parent.ID
}

// GetCache is used for thread-safe access to the cache field
func (d *DriveItem) GetCache() *Cache {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.cache
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

// Statfs returns information about the filesystem. Mainly useful for checking
// quotas and storage limits.
func (d *DriveItem) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	log.WithFields(log.Fields{"path": d.Path()}).Debug()
	resp, err := Get("/me/drive", d.GetCache().GetAuth())
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Could not fetch filesystem details.")
	}
	drive := Drive{}
	json.Unmarshal(resp, &drive)

	if drive.DriveType == "personal" {
		log.Warn("Personal OneDrive accounts do not show number of files, " +
			"inode counts reported by onedriver will be bogus.")
	}

	// limits are pasted from https://support.microsoft.com/en-us/help/3125202
	const blkSize uint64 = 4096 // default ext4 block size
	out.Bsize = uint32(blkSize)
	out.Blocks = drive.Quota.Total / blkSize
	out.Bfree = drive.Quota.Remaining / blkSize
	out.Bavail = drive.Quota.Remaining / blkSize
	out.Files = 100000
	out.Ffree = 100000 - drive.Quota.FileCount
	out.NameLen = 260
	return 0
}

// Readdir returns a list of directory entries (formerly OpenDir).
func (d *DriveItem) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	log.WithFields(log.Fields{
		"path": d.Path(),
		"id":   d.ID(),
	}).Debug()

	cache := d.GetCache()
	// directories are always created with a remote graph id
	children, err := cache.GetChildrenID(d.ID(), cache.GetAuth())
	if err != nil {
		// not an item not found error (Lookup/Getattr will always be called
		// before Readdir()), something has happened to our connection
		log.WithFields(log.Fields{
			"path": d.Path(),
			"err":  err,
		}).Error("Error during Readdir()")
		return nil, syscall.EREMOTEIO
	}

	entries := make([]fuse.DirEntry, 0)
	for _, child := range children {
		entry := fuse.DirEntry{
			Name: child.Name(),
			Mode: child.Mode(),
		}
		entries = append(entries, entry)
	}
	return fs.NewListDirStream(entries), 0
}

// Lookup an individual child of an inode.
func (d *DriveItem) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if ignore(name) {
		return nil, syscall.ENOENT
	}
	log.WithFields(log.Fields{
		"path": d.Path(),
		"id":   d.ID(),
		"name": name,
	}).Trace()

	cache := d.GetCache()
	child, _ := cache.GetChild(d.ID(), strings.ToLower(name), cache.GetAuth())
	if child == nil {
		return nil, syscall.ENOENT
	}
	out.Attr = child.makeattr()
	return d.NewInode(ctx, child, fs.StableAttr{Mode: child.Mode() & fuse.S_IFDIR}), 0
}

// RemoteID uploads an empty file to obtain a Onedrive ID if it doesn't already
// have one. This is necessary to avoid race conditions against uploads if the
// file has not already been uploaded. You can use an empty Auth object if
// you're sure that the item already has an ID or otherwise don't need to fetch
// an ID (such as when deleting an item that is only local).
//TODO move to cache methods
func (d *DriveItem) RemoteID(auth *Auth) (string, error) {
	if d.IsDir() {
		// Directories are always created with an ID. (And this method is only
		// really used for files anyways...)
		return d.ID(), nil
	}

	originalID := d.ID()
	if isLocalID(originalID) && auth.AccessToken != "" {
		uploadPath := fmt.Sprintf("/me/drive/items/%s:/%s:/content", d.ParentID(), d.Name())
		resp, err := Put(uploadPath, auth, strings.NewReader(""))
		if err != nil {
			if strings.Contains(err.Error(), "nameAlreadyExists") {
				// This likely got fired off just as an initial upload completed.
				// Check both our local copy and the server.

				// Do we have it (from another thread)?
				if id := d.ID(); !isLocalID(id) {
					return id, nil
				}

				// Does the server have it?
				latest, err := GetItem(d.Path(), auth)
				if err == nil {
					// hooray!
					err := d.GetCache().MoveID(originalID, latest.IDInternal)
					return latest.IDInternal, err
				}
			}
			// failed to obtain an ID, return whatever it was beforehand
			return originalID, err
		}

		// we use a new DriveItem to unmarshal things into or it will fuck
		// with the existing object (namely its size)
		unsafe := NewDriveItem(d.Name(), 0644, nil)
		err = json.Unmarshal(resp, unsafe)
		if err != nil {
			return originalID, err
		}
		// this is all we really wanted from this transaction
		newID := unsafe.ID()
		err = d.GetCache().MoveID(originalID, newID)
		return newID, err
	}
	return originalID, nil
}

// Path returns an item's full Path
func (d *DriveItem) Path() string {
	// special case when it's the root item
	name := d.Name()
	if d.ParentID() == "" && name == "root" {
		return "/"
	}

	// all paths come prefixed with "/drive/root:"
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if d.APIItem.Parent == nil {
		return name
	}
	prepath := strings.TrimPrefix(d.APIItem.Parent.Path+"/"+name, "/drive/root:")
	return strings.Replace(prepath, "//", "/", -1)
}

// Read from a DriveItem like a file
func (d *DriveItem) Read(ctx context.Context, f fs.FileHandle, buf []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + int(len(buf))
	oend := end
	size := int(d.Size())
	if int(off) > size {
		log.WithFields(log.Fields{
			"id":        d.ID(),
			"path":      d.Path(),
			"bufsize":   int64(end) - off,
			"file_size": size,
			"offset":    off,
		}).Error("Offset was beyond file end (Onedrive metadata was wrong)! " +
			"Refusing op to avoid a segfault.")
		return fuse.ReadResultData(make([]byte, 0)), syscall.EINVAL
	}
	if end > size {
		// d.Size() called once for one fewer RLock
		end = size
	}
	log.WithFields(log.Fields{
		"id":               d.ID(),
		"path":             d.Path(),
		"original_bufsize": int64(oend) - off,
		"bufsize":          int64(end) - off,
		"file_size":        size,
		"offset":           off,
	}).Trace("Read file")

	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return fuse.ReadResultData((*d.data)[off:end]), 0
}

// Write to a DriveItem like a file. Note that changes are 100% local until
// Flush() is called.
func (d *DriveItem) Write(ctx context.Context, f fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	nWrite := len(data)
	offset := int(off)
	log.WithFields(log.Fields{
		"id":      d.ID(),
		"path":    d.Path(),
		"bufsize": nWrite,
		"offset":  off,
	}).Tracef("Write file")

	d.mutex.Lock()
	defer d.mutex.Unlock()
	if offset+nWrite > int(d.SizeInternal)-1 {
		// we've exceeded the file size, overwrite via append
		*d.data = append((*d.data)[:offset], data...)
	} else {
		// writing inside the current file, overwrite in place
		copy((*d.data)[offset:], data)
	}
	// probably a better way to do this, but whatever
	d.SizeInternal = uint64(len(*d.data))
	d.hasChanges = true

	return uint32(nWrite), 0
}

// HasContent returns whether the file has been populated with data
func (d *DriveItem) HasContent() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.data != nil
}

// Flush is called when a file descriptor is closed. This is responsible for all
// uploads of file contents.
func (d *DriveItem) Flush(ctx context.Context, f fs.FileHandle) syscall.Errno {
	log.WithFields(log.Fields{
		"path": d.Path(),
		"id":   d.ID(),
	}).Debug()
	d.mutex.Lock()
	if d.hasChanges {
		d.hasChanges = false
		d.mutex.Unlock()

		// ensureID() is no longer used here to make upload dispatch even faster
		// (since upload is using ensureID() internally)
		cache := d.GetCache()
		if cache == nil {
			log.WithFields(log.Fields{
				"id":   d.ID(),
				"name": d.Name(),
			}).Error("Driveitem cache ref cannot be nil!")
			return syscall.ENODATA
		}
		go d.Upload(cache.auth)
		return 0
	}
	d.mutex.Unlock()
	return 0
}

func (d *DriveItem) makeattr() fuse.Attr {
	mtime := d.ModTime()
	return fuse.Attr{
		Size:  d.Size(),
		Nlink: d.NLink(),
		Mtime: mtime,
		Atime: mtime,
		Ctime: mtime,
		Mode:  d.Mode(),
		Owner: fuse.Owner{
			Uid: uint32(os.Getuid()),
			Gid: uint32(os.Getgid()),
		},
	}
}

// Getattr returns a the DriveItem as a UNIX stat. Holds the read mutex for all
// of the "metadata fetch" operations.
func (d *DriveItem) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	log.WithFields(log.Fields{
		"path": d.Path(),
		"id":   d.ID(),
	}).Trace()
	out.Attr = d.makeattr()
	return 0
}

// Setattr is the workhorse for setting filesystem attributes. Does the work of
// operations like Utimens, Chmod, Chown (not implemented), and Truncate.
func (d *DriveItem) Setattr(ctx context.Context, f fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	log.WithFields(log.Fields{
		"path": d.Path(),
		"id":   d.ID(),
	}).Trace()

	isDir := d.IsDir() // holds an rlock
	d.mutex.Lock()

	// utimens
	if mtime, valid := in.GetMTime(); valid {
		d.ModTimeInternal = &mtime
	}

	// chmod
	if mode, valid := in.GetMode(); valid {
		if isDir {
			d.mode = fuse.S_IFDIR | mode
		} else {
			d.mode = fuse.S_IFREG | mode
		}
	}

	// truncate
	if size, valid := in.GetSize(); valid {
		if size > d.SizeInternal {
			// unlikely to be hit, but implementing just in case
			extra := make([]byte, size-d.SizeInternal)
			*d.data = append(*d.data, extra...)
		} else {
			*d.data = (*d.data)[:size]
		}
		d.SizeInternal = size
		d.hasChanges = true
	}

	d.mutex.Unlock()
	out.Attr = d.makeattr()
	return 0
}

// IsDir returns if it is a directory (true) or file (false).
func (d *DriveItem) IsDir() bool {
	// following statement returns 0 if the dir bit is not set
	return d.Mode()&fuse.S_IFDIR > 0
}

// Mode returns the permissions/mode of the file.
func (d *DriveItem) Mode() uint32 {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if d.mode == 0 { // only 0 if fetched from Graph API
		if d.Folder != nil {
			return fuse.S_IFDIR | 0755
		}
		return fuse.S_IFREG | 0644
	}
	return d.mode
}

// ModTime returns the Unix timestamp of last modification (to get a time.Time
// struct, use time.Unix(int64(d.ModTime()), 0))
func (d *DriveItem) ModTime() uint64 {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return uint64(d.ModTimeInternal.Unix())
}

// NLink gives the number of hard links to an inode (or child count if a
// directory)
func (d *DriveItem) NLink() uint32 {
	if d.IsDir() {
		d.mutex.RLock()
		defer d.mutex.RUnlock()
		// we precompute d.subdir due to mutex lock contention with NLink and
		// other ops. d.subdir is modified by cache Insert/Delete and GetChildren.
		return 2 + d.subdir
	}
	return 1
}

// Size pretends that folders are 4096 bytes, even though they're 0 (since
// they actually don't exist).
func (d *DriveItem) Size() uint64 {
	if d.IsDir() {
		return 4096
	}
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.SizeInternal
}

// these files will never exist, and we should ignore them
func ignore(path string) bool {
	ignoredFiles := []string{
		"BDMV",
		".Trash",
		".Trash-1000",
		".xdg-volume-info",
		"autorun.inf",
		".localized",
		".DS_Store",
		"._.",
		".hidden",
	}
	for _, ignore := range ignoredFiles {
		if path == ignore {
			return true
		}
	}
	return false
}

func octal(i uint32) string {
	return strconv.FormatUint(uint64(i), 8)
}

// Create a new local file. The server doesn't have this yet. The uint32 part of
// the return are fuseflags.
func (d *DriveItem) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	path := d.Path()
	log.WithFields(log.Fields{
		"path": path,
		"name": name,
		"mode": octal(mode),
	}).Debug()

	item := NewDriveItem(name, mode, d)
	d.GetCache().InsertChild(d.ID(), item)
	return d.NewInode(ctx, item, fs.StableAttr{Mode: fuse.S_IFREG}), nil, uint32(0), 0
}

// Mkdir creates a directory.
func (d *DriveItem) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	log.WithFields(log.Fields{
		"path": d.Path(),
		"name": name,
		"mode": octal(mode),
	}).Debug()
	cache := d.GetCache()
	auth := cache.GetAuth()

	// create a new folder on the server
	item, err := Mkdir(name, d.ID(), auth)
	if err != nil {
		log.WithFields(log.Fields{
			"path": name,
			"err":  err,
		}).Error("Error during directory creation:")
		return nil, syscall.EREMOTEIO
	}
	cache.InsertChild(d.ID(), item)
	return d.NewInode(ctx, item, fs.StableAttr{Mode: fuse.S_IFDIR}), 0
}

// Unlink a child file.
func (d *DriveItem) Unlink(ctx context.Context, name string) syscall.Errno {
	log.WithFields(log.Fields{
		"path": d.Path(),
		"id":   d.ID(),
		"name": name,
	}).Debug("Unlinking inode.")

	cache := d.GetCache()
	child, _ := cache.GetChild(d.ID(), name, nil)
	if child == nil {
		// the file we are unlinking never existed
		return syscall.ENOENT
	}

	// if no ID, the item is local-only, and does not need to be deleted on the
	// server
	id := child.ID()
	if !isLocalID(id) {
		if err := Remove(id, cache.GetAuth()); err != nil {
			log.WithFields(log.Fields{
				"err":  err,
				"id":   id,
				"path": d.Path(),
			}).Error("Failed to delete item on server. Aborting op.")
			return syscall.EREMOTEIO
		}
	}

	cache.DeleteID(id)
	return 0
}

// Rmdir deletes a child directory. Reuses Unlink.
func (d *DriveItem) Rmdir(ctx context.Context, name string) syscall.Errno {
	return d.Unlink(ctx, name)
}

// Rename renames an inode.
func (d *DriveItem) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	// we don't fully trust DriveItem.Parent.Path from the Graph API
	cache := d.GetCache()
	path := filepath.Join(cache.InodePath(d.EmbeddedInode()), name)
	dest := filepath.Join(cache.InodePath(newParent.EmbeddedInode()), newName)
	log.WithFields(log.Fields{
		"path": path,
		"dest": dest,
		"id":   d.ID(),
	}).Debug("Renaming inode.")

	auth := cache.GetAuth()
	item, _ := cache.GetChild(d.ID(), name, auth)
	id, err := item.RemoteID(auth)
	if isLocalID(id) || err != nil {
		// uploads will fail without an id
		log.WithFields(log.Fields{
			"id":   id,
			"path": path,
			"err":  err,
		}).Error("ID of item to move cannot be local and we failed to obtain an ID.")
		return syscall.EBADF
	}

	// perform remote rename
	newParentItem, err := cache.GetPath(filepath.Dir(dest), auth)
	if err != nil {
		log.WithFields(log.Fields{
			"path": filepath.Dir(dest),
			"err":  err,
		}).Error("Failed to fetch new parent item by path.")
		return syscall.ENOENT
	}

	parentID := newParentItem.ID()
	if isLocalID(parentID) || err != nil {
		// should never be reached, but being extra safe here
		log.WithFields(log.Fields{
			"id":   parentID,
			"path": filepath.Dir(dest),
			"err":  err,
		}).Error("ID of destination folder cannot be local.")
		return syscall.EBADF
	}

	if err = Rename(id, filepath.Base(dest), parentID, auth); err != nil {
		log.WithFields(log.Fields{
			"id":       id,
			"parentID": parentID,
			"err":      err,
		}).Error("Failed to rename remote item.")
		return syscall.EREMOTEIO
	}

	// now rename local copy
	if err = cache.MovePath(path, dest, auth); err != nil {
		log.WithFields(log.Fields{
			"path": path,
			"dest": dest,
			"err":  err,
		}).Error("Failed to rename local item.")
		return syscall.EIO
	}

	// whew! item renamed
	return 0
}

// Open fetches a DriveItem's content and initializes the .Data field with
// actual data from the server. Data is loaded into memory on Open, and
// persisted to disk on Flush.
func (d *DriveItem) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	path := d.Path()
	id := d.ID()
	log.WithFields(log.Fields{
		"path": path,
		"id":   id,
	}).Debug("Opening file for I/O.")

	if d.HasContent() {
		// we already have data, likely the file is already opened somewhere
		return nil, uint32(0), 0
	}

	// try grabbing from disk
	cache := d.GetCache()
	if content := cache.GetContent(id); content != nil {
		// TODO should verify from cache using hash from server
		log.WithFields(log.Fields{
			"path": path,
			"id":   id,
		}).Info("Found content in cache.")

		d.mutex.Lock()
		defer d.mutex.Unlock()
		// this check is here in case the API file sizes are WRONG (it happens)
		d.SizeInternal = uint64(len(content))
		d.data = &content
		return nil, uint32(0), 0
	}

	// didn't have it on disk, now try api
	log.WithFields(log.Fields{
		"path": path,
	}).Info("Fetching remote content for item from API")

	auth := cache.GetAuth()
	id, err := d.RemoteID(auth)
	if err != nil || id == "" {
		log.WithFields(log.Fields{
			"id":   id,
			"path": path,
			"err":  err,
		}).Error("Could not obtain remote ID.")
		return nil, uint32(0), syscall.EREMOTEIO
	}

	body, err := GetItemContent(id, auth)
	if err != nil {
		log.WithFields(log.Fields{
			"err":  err,
			"id":   id,
			"path": path,
		}).Error("Failed to fetch remote content")
		return nil, uint32(0), syscall.EREMOTEIO
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	// this check is here in case the API file sizes are WRONG (it happens)
	d.SizeInternal = uint64(len(body))
	d.data = &body
	return nil, uint32(0), 0
}
