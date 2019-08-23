package graph

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
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

// DriveItem represents a file or folder fetched from the Graph API. All struct
// fields are pointers so as to avoid including them when marshaling to JSON
// if not present. Fields named "xxxxxInternal" should never be accessed, they
// are there for JSON umarshaling/marshaling only. (They are not safe to access
// concurrently.) This struct's methods are thread-safe and can be called
// concurrently.
type DriveItem struct {
	nodefs.File      `json:"-"`
	cache            *Cache
	uploadSession    *UploadSession   // current upload session, or nil
	data             *[]byte          // empty by default
	hasChanges       bool             // used to trigger an upload on flush
	IDInternal       string           `json:"id,omitempty"`
	NameInternal     string           `json:"name,omitempty"`
	SizeInternal     uint64           `json:"size,omitempty"`
	ModTimeInternal  *time.Time       `json:"lastModifiedDatetime,omitempty"`
	mode             uint32           // do not set manually
	Parent           *DriveItemParent `json:"parentReference,omitempty"`
	children         []string         // a slice of ids, nil when uninitialized
	subdir           uint32           // used purely by NLink()
	mutex            *mu.RWMutex
	Folder           *Folder  `json:"folder,omitempty"`
	FileInternal     *File    `json:"file,omitempty"`
	Deleted          *Deleted `json:"deleted,omitempty"`
	ConflictBehavior string   `json:"@microsoft.graph.conflictBehavior,omitempty"`
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
		File:            nodefs.NewDefaultFile(),
		IDInternal:      localID(),
		NameInternal:    name,
		Parent:          itemParent,
		children:        make([]string, 0),
		mutex:           &mu.RWMutex{},
		data:            &empty,
		ModTimeInternal: &currentTime,
		mode:            mode,
	}
}

// String is only used for debugging by go-fuse
func (d DriveItem) String() string {
	return d.Name()
}

// Name is used to ensure thread-safe access to the NameInternal field.
func (d DriveItem) Name() string {
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
func (d DriveItem) ID() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.IDInternal
}

// ParentID returns the ID of this item's parent.
func (d DriveItem) ParentID() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if d.Parent == nil {
		return ""
	}
	return d.Parent.ID
}

// RemoteID uploads an empty file to obtain a Onedrive ID if it doesn't already
// have one. This is necessary to avoid race conditions against uploads if the
// file has not already been uploaded. You can use an empty Auth object if
// you're sure that the item already has an ID or otherwise don't need to fetch
// an ID (such as when deleting an item that is only local).
//TODO: move this to cache methods, it's not needed here
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
					err := d.cache.MoveID(originalID, latest.IDInternal)
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
		err = d.cache.MoveID(originalID, unsafe.IDInternal)
		return unsafe.IDInternal, err
	}
	return originalID, nil
}

// Path returns an item's full Path
func (d DriveItem) Path() string {
	// special case when it's the root item
	name := d.Name()
	if d.ParentID() == "" && name == "root" {
		return "/"
	}

	// all paths come prefixed with "/drive/root:"
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if d.Parent == nil {
		return name
	}
	prepath := strings.TrimPrefix(d.Parent.Path+"/"+name, "/drive/root:")
	return strings.Replace(prepath, "//", "/", -1)
}

// FetchContent fetches a DriveItem's content and initializes the .Data field.
func (d *DriveItem) FetchContent(auth *Auth) error {
	id, err := d.RemoteID(auth)
	if err != nil {
		log.WithFields(log.Fields{
			"id":   d.ID(),
			"name": d.Name(),
			"err":  err,
		}).Error("Could not obtain remote ID.")
		return err
	}
	body, err := Get("/me/drive/items/"+id+"/content", auth)
	if err != nil {
		return err
	}
	d.mutex.Lock()
	// this check is here in case the onedrive file sizes are WRONG.
	// (it happens)
	d.SizeInternal = uint64(len(body))
	d.data = &body
	d.File = nodefs.NewDefaultFile()
	d.mutex.Unlock()
	return nil
}

// Read from a DriveItem like a file
func (d DriveItem) Read(buf []byte, off int64) (fuse.ReadResult, fuse.Status) {
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
		return fuse.ReadResultData(make([]byte, 0)), fuse.EINVAL
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
	return fuse.ReadResultData((*d.data)[off:end]), fuse.OK
}

// Write to a DriveItem like a file. Note that changes are 100% local until
// Flush() is called.
func (d *DriveItem) Write(data []byte, off int64) (uint32, fuse.Status) {
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

	return uint32(nWrite), fuse.OK
}

// Flush is called when a file descriptor is closed. This is responsible for all
// uploads of file contents.
func (d *DriveItem) Flush() fuse.Status {
	log.WithFields(log.Fields{"path": d.Path()}).Debug()
	d.mutex.Lock()
	if d.hasChanges {
		d.hasChanges = false
		// ensureID() is no longer used here to make upload dispatch even faster
		// (since upload is using ensureID() internally)
		if d.cache == nil {
			d.mutex.Unlock() // must unlock for log to work
			log.WithFields(log.Fields{
				"id":   d.ID(),
				"name": d.Name(),
			}).Error("Driveitem cache ref cannot be nil!")
			return fuse.ENODATA
		}
		auth := d.cache.auth
		d.mutex.Unlock()
		go d.Upload(auth)
		return fuse.OK
	}
	d.mutex.Unlock()
	return fuse.OK
}

// GetAttr returns a the DriveItem as a UNIX stat. Holds the read mutex for all
// of the "metadata fetch" operations.
func (d DriveItem) GetAttr(out *fuse.Attr) fuse.Status {
	out.Size = d.Size()
	out.Nlink = d.NLink()
	out.Mtime = d.ModTime()
	out.Atime = out.Mtime
	out.Ctime = out.Mtime
	out.Mode = d.Mode()
	out.Owner = fuse.Owner{
		Uid: uint32(os.Getuid()),
		Gid: uint32(os.Getgid()),
	}
	return fuse.OK
}

// Utimens sets the access/modify times of a file
func (d *DriveItem) Utimens(atime *time.Time, mtime *time.Time) fuse.Status {
	log.WithFields(log.Fields{"path": d.Path()}).Trace()
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.ModTimeInternal = mtime
	return fuse.OK
}

// Truncate cuts a file in place
func (d *DriveItem) Truncate(size uint64) fuse.Status {
	log.WithFields(log.Fields{"path": d.Path()}).Debug()
	d.mutex.Lock()
	defer d.mutex.Unlock()
	*d.data = (*d.data)[:size]
	d.SizeInternal = size
	d.hasChanges = true
	return fuse.OK
}

// IsDir returns if it is a directory (true) or file (false).
func (d DriveItem) IsDir() bool {
	// following statement returns 0 if the dir bit is not set
	return d.Mode()&fuse.S_IFDIR > 0
}

// Mode returns the permissions/mode of the file.
func (d DriveItem) Mode() uint32 {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if d.mode == 0 { // only 0 if fetched from Graph API
		if d.FileInternal == nil { // nil if a folder
			return fuse.S_IFDIR | 0755
		}
		return fuse.S_IFREG | 0644
	}
	return d.mode
}

// Chmod changes the mode of a file
func (d *DriveItem) Chmod(perms uint32) fuse.Status {
	log.WithFields(log.Fields{"path": d.Path()}).Debug()
	isDir := d.IsDir() // holds an rlock
	d.mutex.Lock()
	if isDir {
		d.mode = fuse.S_IFDIR | perms
	} else {
		d.mode = fuse.S_IFREG | perms
	}
	d.mutex.Unlock()
	return fuse.OK
}

// ModTime returns the Unix timestamp of last modification (to get a time.Time
// struct, use time.Unix(int64(d.ModTime()), 0))
func (d DriveItem) ModTime() uint64 {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return uint64(d.ModTimeInternal.Unix())
}

// NLink gives the number of hard links to an inode (or child count if a
// directory)
func (d DriveItem) NLink() uint32 {
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
func (d DriveItem) Size() uint64 {
	if d.IsDir() {
		return 4096
	}
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.SizeInternal
}
