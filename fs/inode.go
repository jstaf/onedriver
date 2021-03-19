package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
	log "github.com/sirupsen/logrus"
)

// Inode represents a file or folder fetched from the Graph API. All struct
// fields are pointers so as to avoid including them when marshaling to JSON
// if not present. The embedded DriveItem's fields should never be accessed, they
// are there for JSON umarshaling/marshaling only. (They are not safe to access
// concurrently.) This struct's methods are thread-safe and can be called
// concurrently. Reads/writes are done directly to DriveItems instead of
// implementing something like the fs.FileHandle to minimize the complexity of
// operations like Flush.
type Inode struct {
	fs.Inode `json:"-"`

	mutex sync.RWMutex // used to be a pointer, but fs.Inode also embeds a mutex :(
	graph.DriveItem
	cache         *Cache
	children      []string       // a slice of ids, nil when uninitialized
	uploadSession *UploadSession // current upload session, or nil
	data          *[]byte        // empty by default
	hasChanges    bool           // used to trigger an upload on flush
	subdir        uint32         // used purely by NLink()
	mode          uint32         // do not set manually
}

// SerializeableInode is like a Inode, but can be serialized for local storage
// to disk
type SerializeableInode struct {
	graph.DriveItem
	Children []string
	Subdir   uint32
	Mode     uint32
}

// NewInode initializes a new Inode
func NewInode(name string, mode uint32, parent *Inode) *Inode {
	itemParent := &graph.DriveItemParent{ID: "", Path: ""}
	if parent != nil {
		itemParent.Path = parent.Path()
		parent.mutex.RLock()
		itemParent.ID = parent.DriveItem.ID
		itemParent.DriveID = parent.DriveItem.Parent.DriveID
		itemParent.DriveType = parent.DriveItem.Parent.DriveType
		parent.mutex.RUnlock()
	}

	var empty []byte
	currentTime := time.Now()
	return &Inode{
		DriveItem: graph.DriveItem{
			ID:      localID(),
			Name:    name,
			Parent:  itemParent,
			ModTime: &currentTime,
		},
		children: make([]string, 0),
		data:     &empty,
		mode:     mode,
	}
}

// AsJSON converts a DriveItem to JSON for use with local storage. Not used with
// the API. FIXME: If implemented as MarshalJSON, this will break delta syncs
// for business accounts. Don't ask me why.
func (i *Inode) AsJSON() []byte {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	data, _ := json.Marshal(SerializeableInode{
		DriveItem: i.DriveItem,
		Children:  i.children,
		Subdir:    i.subdir,
		Mode:      i.mode,
	})
	return data
}

// NewInodeJSON converts JSON to a *DriveItem when loading from local storage. Not
// used with the API. FIXME: If implemented as UnmarshalJSON, this will break
// delta syncs for business accounts. Don't ask me why.
func NewInodeJSON(data []byte) (*Inode, error) {
	var raw SerializeableInode
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &Inode{
		DriveItem: raw.DriveItem,
		children:  raw.Children,
		mode:      raw.Mode,
		subdir:    raw.Subdir,
	}, nil
}

// NewInodeDriveItem creates a new DriveItem from an Inode
func NewInodeDriveItem(item *graph.DriveItem) *Inode {
	if item == nil {
		return nil
	}
	return &Inode{
		DriveItem: *item,
	}
}

// String is only used for debugging by go-fuse
func (i *Inode) String() string {
	return i.Name()
}

// Name is used to ensure thread-safe access to the NameInternal field.
func (i *Inode) Name() string {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return i.DriveItem.Name
}

// SetName sets the name of the item in a thread-safe manner.
func (i *Inode) SetName(name string) {
	i.mutex.Lock()
	i.DriveItem.Name = name
	i.mutex.Unlock()
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
func (i *Inode) ID() string {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return i.DriveItem.ID
}

// ParentID returns the ID of this item's parent.
func (i *Inode) ParentID() string {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	if i.DriveItem.Parent == nil {
		return ""
	}
	return i.DriveItem.Parent.ID
}

// GetCache is used for thread-safe access to the cache field
func (i *Inode) GetCache() *Cache {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return i.cache
}

// Statfs returns information about the filesystem. Mainly useful for checking
// quotas and storage limits.
func (i *Inode) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	log.WithFields(log.Fields{"path": i.Path()}).Debug()
	drive, err := graph.GetDrive(i.GetCache().GetAuth())
	if err != nil {
		return syscall.EREMOTEIO
	}

	if drive.DriveType == graph.DriveTypePersonal {
		log.Warn("Personal OneDrive accounts do not show number of files, " +
			"inode counts reported by onedriver will be bogus.")
	} else if drive.Quota.Total == 0 { // <-- check for if microsoft ever fixes their API
		log.Warn("OneDrive for Business accounts do not report quotas, " +
			"pretending the quota is 5TB and it's all unused.")
		drive.Quota.Total = 5 * uint64(math.Pow(1024, 4))
		drive.Quota.Remaining = 5 * uint64(math.Pow(1024, 4))
		drive.Quota.FileCount = 0
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
func (i *Inode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	log.WithFields(log.Fields{
		"path": i.Path(),
		"id":   i.ID(),
	}).Debug()

	cache := i.GetCache()
	// directories are always created with a remote graph id
	children, err := cache.GetChildrenID(i.ID(), cache.GetAuth())
	if err != nil {
		// not an item not found error (Lookup/Getattr will always be called
		// before Readdir()), something has happened to our connection
		log.WithFields(log.Fields{
			"path": i.Path(),
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
func (i *Inode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	log.WithFields(log.Fields{
		"path": i.Path(),
		"id":   i.ID(),
		"name": name,
	}).Trace()

	cache := i.GetCache()
	child, _ := cache.GetChild(i.ID(), strings.ToLower(name), cache.GetAuth())
	if child == nil {
		return nil, syscall.ENOENT
	}
	out.Attr = child.makeattr()
	return i.NewInode(ctx, child, fs.StableAttr{Mode: child.Mode() & fuse.S_IFDIR}), 0
}

// RemoteID uploads an empty file to obtain a Onedrive ID if it doesn't already
// have one. This is necessary to avoid race conditions against uploads if the
// file has not already been uploaded. You can use an empty Auth object if
// you're sure that the item already has an ID or otherwise don't need to fetch
// an ID (such as when deleting an item that is only local).
func (i *Inode) RemoteID(auth *graph.Auth) (string, error) {
	if i.IsDir() {
		// Directories are always created with an ID. (And this method is only
		// really used for files anyways...)
		return i.ID(), nil
	}

	originalID := i.ID()
	if isLocalID(originalID) && auth.AccessToken != "" {
		uploadPath := fmt.Sprintf(
			"/me/drive/items/%s:/%s:/content",
			i.ParentID(),
			url.PathEscape(i.Name()),
		)
		resp, err := graph.Put(uploadPath, auth, strings.NewReader(""))
		if err != nil {
			if strings.Contains(err.Error(), "nameAlreadyExists") {
				// This likely got fired off just as an initial upload completed.
				// Check both our local copy and the server.

				// Do we have it (from another thread)?
				id := i.ID()
				if !isLocalID(id) {
					return id, nil
				}

				// Does the server have it?
				latest, err := graph.GetItem(id, auth)
				if err == nil {
					// hooray!
					err := i.GetCache().MoveID(originalID, latest.ID)
					return latest.ID, err
				}
			}
			// failed to obtain an ID, return whatever it was beforehand
			return originalID, err
		}

		// we use a new DriveItem to unmarshal things into or it will fuck
		// with the existing object (namely its size)
		name := i.Name()
		unsafe := NewInode(name, 0644, nil)
		err = json.Unmarshal(resp, unsafe)
		if err != nil {
			return originalID, err
		}
		// this is all we really wanted from this transaction
		newID := unsafe.ID()
		err = i.GetCache().MoveID(originalID, newID)
		log.WithFields(log.Fields{
			"name":     name,
			"original": originalID,
			"new":      newID,
		}).Info("Exchanged ID.")
		return newID, err
	}
	return originalID, nil
}

// Path returns an inode's full Path
func (i *Inode) Path() string {
	// special case when it's the root item
	name := i.Name()
	if i.ParentID() == "" && name == "root" {
		return "/"
	}

	// all paths come prefixed with "/drive/root:"
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	if i.DriveItem.Parent == nil {
		return name
	}
	prepath := strings.TrimPrefix(i.DriveItem.Parent.Path+"/"+name, "/drive/root:")
	return strings.Replace(prepath, "//", "/", -1)
}

// Read from an Inode like a file
func (i *Inode) Read(ctx context.Context, f fs.FileHandle, buf []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	path := i.Path()
	if !i.HasContent() {
		log.WithFields(log.Fields{
			"id":   i.ID(),
			"path": path,
		}).Warn("Read called on a closed file descriptor! Reopening file for op.")
		i.Open(ctx, 0)
	}

	// we are locked for the remainder of this op
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	end := int(off) + int(len(buf))
	oend := end
	size := len(*i.data) // worse than using i.size(), but some edge cases require it
	if int(off) > size {
		log.WithFields(log.Fields{
			"id":        i.DriveItem.ID,
			"path":      path,
			"bufsize":   int64(end) - off,
			"file_size": size,
			"offset":    off,
		}).Error("Offset was beyond file end (Onedrive metadata was wrong!). Refusing op.")
		return fuse.ReadResultData(make([]byte, 0)), syscall.EINVAL
	}
	if end > size {
		end = size
	}
	log.WithFields(log.Fields{
		"id":               i.DriveItem.ID,
		"path":             path,
		"original_bufsize": int64(oend) - off,
		"bufsize":          int64(end) - off,
		"file_size":        size,
		"offset":           off,
	}).Trace("Read file")
	return fuse.ReadResultData((*i.data)[off:end]), 0
}

// Write to an Inode like a file. Note that changes are 100% local until
// Flush() is called.
func (i *Inode) Write(ctx context.Context, f fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	nWrite := len(data)
	offset := int(off)
	log.WithFields(log.Fields{
		"id":      i.ID(),
		"path":    i.Path(),
		"bufsize": nWrite,
		"offset":  off,
	}).Tracef("Write file")

	if !i.HasContent() {
		log.WithFields(log.Fields{
			"id":   i.ID(),
			"path": i.Path(),
		}).Warn("Write called on a closed file descriptor! Reopening file for write op.")
		i.Open(ctx, 0)
	}

	i.mutex.Lock()
	defer i.mutex.Unlock()
	if offset+nWrite > int(i.DriveItem.Size)-1 {
		// we've exceeded the file size, overwrite via append
		*i.data = append((*i.data)[:offset], data...)
	} else {
		// writing inside the current file, overwrite in place
		copy((*i.data)[offset:], data)
	}
	// probably a better way to do this, but whatever
	i.DriveItem.Size = uint64(len(*i.data))
	i.hasChanges = true

	return uint32(nWrite), 0
}

// HasContent returns whether the file has been populated with data
func (i *Inode) HasContent() bool {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return i.data != nil
}

// HasChanges returns true if the file has local changes that haven't been
// uploaded yet.
func (i *Inode) HasChanges() bool {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return i.hasChanges
}

// HasChildren returns true if the item has more than 0 children
func (i *Inode) HasChildren() bool {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return len(i.children) > 0
}

// Fsync is a signal to ensure writes to the Inode are flushed to stable
// storage. This method is used to trigger uploads of file content.
func (i *Inode) Fsync(ctx context.Context, f fs.FileHandle, flags uint32) syscall.Errno {
	log.WithFields(log.Fields{
		"id":   i.ID(),
		"path": i.Path(),
	}).Debug()
	if i.HasChanges() {
		i.mutex.Lock()
		i.hasChanges = false

		// recompute hashes when saving new content
		i.DriveItem.File = &graph.File{}
		if i.DriveItem.Parent.DriveType == graph.DriveTypePersonal {
			i.DriveItem.File.Hashes.SHA1Hash = graph.SHA1Hash(i.data)
		} else {
			i.DriveItem.File.Hashes.QuickXorHash = graph.QuickXORHash(i.data)
		}
		i.mutex.Unlock()

		if err := i.cache.uploads.QueueUpload(i); err != nil {
			log.WithFields(log.Fields{
				"id":   i.ID(),
				"name": i.Name(),
				"err":  err,
			}).Error("Error creating upload session.")
			return syscall.EREMOTEIO
		}
		return 0
	}
	return 0
}

// Flush is called when a file descriptor is closed. Uses Fsync to perform file
// uploads.
func (i *Inode) Flush(ctx context.Context, f fs.FileHandle) syscall.Errno {
	log.WithFields(log.Fields{
		"path": i.Path(),
		"id":   i.ID(),
	}).Debug()
	i.Fsync(ctx, f, 0)

	// wipe data from memory to avoid mem bloat over time
	i.mutex.Lock()
	if i.data != nil {
		i.cache.InsertContent(i.DriveItem.ID, *i.data)
		i.data = nil
	}
	i.mutex.Unlock()
	return 0
}

// makeattr a convenience function to create a set of filesystem attrs for use
// with syscalls that use or modify attrs.
func (i *Inode) makeattr() fuse.Attr {
	mtime := i.ModTime()
	return fuse.Attr{
		Size:  i.Size(),
		Nlink: i.NLink(),
		Mtime: mtime,
		Atime: mtime,
		Ctime: mtime,
		Mode:  i.Mode(),
		Owner: fuse.Owner{
			Uid: uint32(os.Getuid()),
			Gid: uint32(os.Getgid()),
		},
	}
}

// Getattr returns a the Inode as a UNIX stat. Holds the read mutex for all of
// the "metadata fetch" operations.
func (i *Inode) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	log.WithFields(log.Fields{
		"path": i.Path(),
		"id":   i.ID(),
	}).Trace()
	out.Attr = i.makeattr()
	return 0
}

// Setattr is the workhorse for setting filesystem attributes. Does the work of
// operations like Utimens, Chmod, Chown (not implemented), and Truncate.
func (i *Inode) Setattr(ctx context.Context, f fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	log.WithFields(log.Fields{
		"path": i.Path(),
		"id":   i.ID(),
	}).Trace()

	isDir := i.IsDir() // holds an rlock
	i.mutex.Lock()

	// utimens
	if mtime, valid := in.GetMTime(); valid {
		i.DriveItem.ModTime = &mtime
	}

	// chmod
	if mode, valid := in.GetMode(); valid {
		if isDir {
			i.mode = fuse.S_IFDIR | mode
		} else {
			i.mode = fuse.S_IFREG | mode
		}
	}

	// truncate
	if size, valid := in.GetSize(); valid {
		if size > i.DriveItem.Size {
			// unlikely to be hit, but implementing just in case
			extra := make([]byte, size-i.DriveItem.Size)
			*i.data = append(*i.data, extra...)
		} else {
			*i.data = (*i.data)[:size]
		}
		i.DriveItem.Size = size
		i.hasChanges = true
	}

	i.mutex.Unlock()
	out.Attr = i.makeattr()
	return 0
}

// IsDir returns if it is a directory (true) or file (false).
func (i *Inode) IsDir() bool {
	// 0 if the dir bit is not set
	return i.Mode()&fuse.S_IFDIR > 0
}

// Mode returns the permissions/mode of the file.
func (i *Inode) Mode() uint32 {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	if i.mode == 0 { // only 0 if fetched from Graph API
		if i.Folder != nil {
			return fuse.S_IFDIR | 0755
		}
		return fuse.S_IFREG | 0644
	}
	return i.mode
}

// ModTime returns the Unix timestamp of last modification (to get a time.Time
// struct, use time.Unix(int64(d.ModTime()), 0))
func (i *Inode) ModTime() uint64 {
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return uint64(i.DriveItem.ModTime.Unix())
}

// NLink gives the number of hard links to an inode (or child count if a
// directory)
func (i *Inode) NLink() uint32 {
	if i.IsDir() {
		i.mutex.RLock()
		defer i.mutex.RUnlock()
		// we precompute subdir due to mutex lock contention between NLink and
		// other ops. subdir is modified by cache Insert/Delete and GetChildren.
		return 2 + i.subdir
	}
	return 1
}

// Size pretends that folders are 4096 bytes, even though they're 0 (since
// they actually don't exist).
func (i *Inode) Size() uint64 {
	if i.IsDir() {
		return 4096
	}
	i.mutex.RLock()
	defer i.mutex.RUnlock()
	return i.DriveItem.Size
}

// Octal converts a number to its octal representation in string form.
func Octal(i uint32) string {
	return strconv.FormatUint(uint64(i), 8)
}

// Create a new local file. The server doesn't have this yet. The uint32 part of
// the return are fuse flags.
func (i *Inode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	path := i.Path()
	id := i.ID()
	cache := i.GetCache()
	if cache.IsOffline() {
		// nope, we are refusing op to avoid data loss later
		log.WithFields(log.Fields{
			"id":   id,
			"path": path,
			"name": name,
		}).Warn("We are offline. Refusing Create() to avoid data loss later.")
		return nil, nil, uint32(0), syscall.EROFS
	}

	inode := NewInode(name, mode, i)
	log.WithFields(log.Fields{
		"id":      id,
		"childid": inode.ID(),
		"path":    path,
		"name":    name,
		"mode":    Octal(mode),
	}).Debug()
	cache.InsertChild(id, inode)
	return i.NewInode(ctx, inode, fs.StableAttr{Mode: fuse.S_IFREG}), nil, uint32(0), 0
}

// Mkdir creates a directory.
func (i *Inode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	log.WithFields(log.Fields{
		"path": i.Path(),
		"name": name,
		"mode": Octal(mode),
	}).Debug()
	cache := i.GetCache()
	auth := cache.GetAuth()

	// create a new folder on the server
	item, err := graph.Mkdir(name, i.ID(), auth)
	if err != nil {
		log.WithFields(log.Fields{
			"path": name,
			"err":  err,
		}).Error("Error during directory creation:")
		return nil, syscall.EREMOTEIO
	}
	inode := NewInodeDriveItem(item)
	cache.InsertChild(i.ID(), inode)
	return i.NewInode(ctx, inode, fs.StableAttr{Mode: fuse.S_IFDIR}), 0
}

// Unlink a child file.
func (i *Inode) Unlink(ctx context.Context, name string) syscall.Errno {
	log.WithFields(log.Fields{
		"path": i.Path(),
		"id":   i.ID(),
		"name": name,
	}).Debug("Unlinking inode.")

	cache := i.GetCache()
	child, _ := cache.GetChild(i.ID(), name, nil)
	if child == nil {
		// the file we are unlinking never existed
		return syscall.ENOENT
	}
	if cache.IsOffline() {
		return syscall.EROFS
	}

	// if no ID, the item is local-only, and does not need to be deleted on the
	// server
	id := child.ID()
	if !isLocalID(id) {
		if err := graph.Remove(id, cache.GetAuth()); err != nil {
			log.WithFields(log.Fields{
				"err":  err,
				"id":   id,
				"path": i.Path(),
			}).Error("Failed to delete item on server. Aborting op.")
			return syscall.EREMOTEIO
		}
	}

	cache.DeleteID(id)
	cache.DeleteContent(id)
	return 0
}

// Rmdir deletes a child directory. Reuses Unlink.
func (i *Inode) Rmdir(ctx context.Context, name string) syscall.Errno {
	return i.Unlink(ctx, name)
}

// Rename renames an inode.
func (i *Inode) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	// we don't fully trust DriveItem.Parent.Path from the Graph API
	cache := i.GetCache()
	path := filepath.Join(cache.InodePath(i.EmbeddedInode()), name)
	dest := filepath.Join(cache.InodePath(newParent.EmbeddedInode()), newName)
	log.WithFields(log.Fields{
		"path": path,
		"dest": dest,
		"id":   i.ID(),
	}).Debug("Renaming inode.")

	auth := cache.GetAuth()
	inode, _ := cache.GetChild(i.ID(), name, auth)
	id, err := inode.RemoteID(auth)
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
	if isLocalID(parentID) {
		// should never be reached, but being extra safe here
		log.WithFields(log.Fields{
			"id":   parentID,
			"path": filepath.Dir(dest),
			"err":  err,
		}).Error("ID of destination folder cannot be local.")
		return syscall.EBADF
	}

	if err = graph.Rename(id, filepath.Base(dest), parentID, auth); err != nil {
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

// Open fetches a Inodes's content and initializes the .Data field with actual
// data from the server. Data is loaded into memory on Open, and persisted to
// disk on Flush.
func (i *Inode) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	path := i.Path()
	id := i.ID()
	f := int(flags)
	if f&os.O_RDWR+f&os.O_WRONLY > 0 && i.GetCache().offline {
		log.WithFields(log.Fields{
			"path":  path,
			"id":    id,
			"flags": flags,
		}).Debug("Refusing Open() with write flag, FS is offline.")
		return nil, uint32(0), syscall.EROFS
	}

	log.WithFields(log.Fields{
		"path": path,
		"id":   id,
	}).Debug("Opening file for I/O.")

	if i.HasContent() {
		// we already have data, likely the file is already opened somewhere
		return nil, uint32(0), 0
	}

	// try grabbing from disk
	cache := i.GetCache()
	if content := cache.GetContent(id); content != nil {
		// verify content against what we're supposed to have
		var hashMatch bool
		i.mutex.RLock()
		driveType := i.DriveItem.Parent.DriveType
		if isLocalID(id) && i.DriveItem.File == nil {
			// only check hashes if the file has been uploaded before, otherwise
			// we just accept the cached content.
			hashMatch = true
		} else if driveType == graph.DriveTypePersonal {
			hashMatch = i.VerifyChecksum(graph.SHA1Hash(&content))
		} else if driveType == graph.DriveTypeBusiness || driveType == graph.DriveTypeSharepoint {
			hashMatch = i.VerifyChecksum(graph.QuickXORHash(&content))
		} else {
			hashMatch = true
			log.WithFields(log.Fields{
				"path":      path,
				"driveType": driveType,
				"id":        id,
			}).Warn("Could not determine drive type, not checking hashes.")
		}
		i.mutex.RUnlock()

		if hashMatch {
			// disk content is only used if the checksums match
			log.WithFields(log.Fields{
				"path": path,
				"id":   id,
			}).Info("Found content in cache.")

			i.mutex.Lock()
			defer i.mutex.Unlock()
			// this check is here in case the API file sizes are WRONG (it happens)
			i.DriveItem.Size = uint64(len(content))
			i.data = &content
			return nil, uint32(0), 0
		}
		log.WithFields(log.Fields{
			"id":        id,
			"path":      path,
			"drivetype": driveType,
		}).Info("Not using cached item due to file hash mismatch.")
	}

	// didn't have it on disk, now try api
	log.WithFields(log.Fields{
		"id":   id,
		"path": path,
	}).Info("Fetching remote content for item from API.")

	auth := cache.GetAuth()
	id, err := i.RemoteID(auth)
	if err != nil || id == "" {
		log.WithFields(log.Fields{
			"id":   id,
			"path": path,
			"err":  err,
		}).Error("Could not obtain remote ID.")
		return nil, uint32(0), syscall.EREMOTEIO
	}

	body, err := graph.GetItemContent(id, auth)
	if err != nil {
		log.WithFields(log.Fields{
			"err":  err,
			"id":   id,
			"path": path,
		}).Error("Failed to fetch remote content.")
		return nil, uint32(0), syscall.EREMOTEIO
	}

	i.mutex.Lock()
	defer i.mutex.Unlock()
	// this check is here in case the API file sizes are WRONG (it happens)
	i.DriveItem.Size = uint64(len(body))
	i.data = &body
	return nil, uint32(0), 0
}
