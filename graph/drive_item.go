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
	"github.com/jstaf/onedriver/logger"
	mu "github.com/sasha-s/go-deadlock"
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
	var cache *Cache
	if parent != nil {
		parent.mutex.RLock()
		itemParent.ID = parent.IDInternal
		itemParent.Path = parent.Path()
		cache = parent.cache
		defer parent.mutex.RUnlock()
	}

	var empty []byte
	currentTime := time.Now()
	return &DriveItem{
		File:            nodefs.NewDefaultFile(),
		IDInternal:      localID(),
		NameInternal:    name,
		cache:           cache, //TODO: find a way to do uploads without this field
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
	//FIXME: using locks here results in a double mutex lock for some ops (such as
	// when name is used inside another op)
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

// RemoteID uploads an empty file to obtain a Onedrive ID if it doesn't already
// have one. This is necessary to avoid race conditions against uploads if the
// file has not already been uploaded. You can use an empty Auth object if
// you're sure that the item already has an ID or otherwise don't need to fetch
// an ID (such as when deleting an item that is only local).
//TODO: move this to cache methods, it's not needed here
func (d *DriveItem) RemoteID(auth *Auth) (string, error) {
	// copy the item so we can access it's ID without locking the item later
	d.mutex.RLock()
	cpy := *d
	parentID := d.Parent.ID
	d.mutex.RUnlock()

	if cpy.IsDir() {
		//TODO add checks for directory, perhaps retry the dir creation again
		//server-side?
		return cpy.IDInternal, nil
	}

	if isLocalID(cpy.IDInternal) && auth.AccessToken != "" {
		uploadPath := fmt.Sprintf("/me/drive/items/%s:/%s:/content", parentID, cpy.Name())
		resp, err := Put(uploadPath, auth, strings.NewReader(""))
		if err != nil {
			if strings.Contains(err.Error(), "nameAlreadyExists") {
				// This likely got fired off just as an initial upload completed.
				// Check both our local copy and the server.

				// Do we have it (from another thread)?
				d.mutex.RLock()
				id := d.IDInternal
				path := d.Path()
				if id != "" {
					defer d.mutex.RUnlock()
					return id, nil
				}
				d.mutex.RUnlock()

				// Does the server have it?
				latest, err := GetItem(path, auth)
				if err == nil {
					// hooray!
					err := d.cache.MoveID(cpy.IDInternal, latest.IDInternal)
					return latest.IDInternal, err
				}
			}
			// failed to obtain an ID, return whatever it was beforehand
			return cpy.IDInternal, err
		}

		// we use a new DriveItem to unmarshal things into or it will fuck
		// with the existing object (namely its size)
		unsafe := NewDriveItem(cpy.Name(), 0644, nil)
		err = json.Unmarshal(resp, unsafe)
		if err != nil {
			return cpy.IDInternal, err
		}
		// this is all we really wanted from this transaction
		err = d.cache.MoveID(cpy.IDInternal, unsafe.IDInternal)
		return unsafe.IDInternal, err
	}
	return cpy.IDInternal, nil
}

// Path returns an item's full Path
func (d DriveItem) Path() string {
	// special case when it's the root item
	if d.Parent.ID == "" && d.Name() == "root" {
		return "/"
	}

	// all paths come prefixed with "/drive/root:"
	prepath := strings.TrimPrefix(d.Parent.Path+"/"+d.Name(), "/drive/root:")
	return strings.Replace(prepath, "//", "/", -1)
}

// FetchContent fetches a DriveItem's content and initializes the .Data field.
func (d *DriveItem) FetchContent(auth *Auth) error {
	id, err := d.RemoteID(auth)
	if err != nil {
		logger.Error("Could not obtain ID:", err.Error())
		return err
	}
	body, err := Get("/me/drive/items/"+id+"/content", auth)
	if err != nil {
		return err
	}
	d.mutex.Lock()
	d.data = &body
	d.File = nodefs.NewDefaultFile()
	d.mutex.Unlock()
	return nil
}

// Read from a DriveItem like a file
func (d DriveItem) Read(buf []byte, off int64) (fuse.ReadResult, fuse.Status) {
	end := int(off) + int(len(buf))
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if end > len(*d.data) {
		end = len(*d.data)
	}
	logger.Tracef("%s: %d bytes at offset %d\n", d.Path(), int64(end)-off, off)
	return fuse.ReadResultData((*d.data)[off:end]), fuse.OK
}

// Write to a DriveItem like a file. Note that changes are 100% local until
// Flush() is called.
func (d *DriveItem) Write(data []byte, off int64) (uint32, fuse.Status) {
	nWrite := len(data)
	offset := int(off)
	logger.Tracef("%s: %d bytes at offset %d\n", d.Path(), nWrite, off)

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
	logger.Trace(d.Path())
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.hasChanges {
		d.hasChanges = false
		// ensureID() is no longer used here to make upload dispatch even faster
		// (since upload is using ensureID() internally)
		if d.cache == nil {
			logger.Error("Driveitem cache ref cannot be nil!", d.Name())
			return fuse.ENODATA
		}
		go d.Upload(d.cache.auth)
	}
	return fuse.OK
}

// GetAttr returns a the DriveItem as a UNIX stat. Holds the read mutex for all
// of the "metadata fetch" operations.
func (d DriveItem) GetAttr(out *fuse.Attr) fuse.Status {
	out.Size = d.Size()
	out.Nlink = d.NLink()
	out.Atime = d.ModTime()
	out.Mtime = d.ModTime()
	out.Ctime = d.ModTime()
	out.Mode = d.Mode()
	out.Owner = fuse.Owner{
		Uid: uint32(os.Getuid()),
		Gid: uint32(os.Getgid()),
	}
	return fuse.OK
}

// Utimens sets the access/modify times of a file
func (d *DriveItem) Utimens(atime *time.Time, mtime *time.Time) fuse.Status {
	logger.Trace(d.Path())
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.ModTimeInternal = mtime
	return fuse.OK
}

// Truncate cuts a file in place
func (d *DriveItem) Truncate(size uint64) fuse.Status {
	logger.Trace(d.Path())
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
	if d.mode == 0 { // only 0 if fetched from Graph API
		if d.FileInternal == nil { // nil if a folder
			d.mode = fuse.S_IFDIR | 0755
		} else {
			d.mode = fuse.S_IFREG | 0644
		}
	}
	return d.mode
}

// Chmod changes the mode of a file
func (d *DriveItem) Chmod(perms uint32) fuse.Status {
	logger.Trace(d.Path())
	d.mutex.Lock()
	if d.IsDir() {
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
