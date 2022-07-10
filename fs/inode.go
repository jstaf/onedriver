package fs

import (
	"encoding/json"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
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
	sync.RWMutex
	graph.DriveItem
	nodeID     uint64   // filesystem node id
	children   []string // a slice of ids, nil when uninitialized
	data       *[]byte  // empty by default
	hasChanges bool     // used to trigger an upload on flush
	subdir     uint32   // used purely by NLink()
	mode       uint32   // do not set manually
}

// SerializeableInode is like a Inode, but can be serialized for local storage
// to disk
type SerializeableInode struct {
	graph.DriveItem
	Children []string
	Subdir   uint32
	Mode     uint32
}

// NewInode initializes a new Inode. mode requires either the fuse.S_IFDIR or
// fuse.S_IFREG bits to be set.
func NewInode(name string, mode uint32, parent *Inode) *Inode {
	itemParent := &graph.DriveItemParent{ID: "", Path: ""}
	if parent != nil {
		itemParent.Path = parent.Path()
		parent.RLock()
		itemParent.ID = parent.DriveItem.ID
		itemParent.DriveID = parent.DriveItem.Parent.DriveID
		itemParent.DriveType = parent.DriveItem.Parent.DriveType
		parent.RUnlock()
	}

	if mode&fuse.S_IFDIR+mode&fuse.S_IFREG == 0 {
		// panic - this is a programming error on our part
		panic("either the fuse.S_IFDIR or fuse.S_IFREG bits must be set")
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
	i.RLock()
	defer i.RUnlock()
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

// NewInodeDriveItem creates a new Inode from a DriveItem
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
	i.RLock()
	defer i.RUnlock()
	return i.DriveItem.Name
}

// SetName sets the name of the item in a thread-safe manner.
func (i *Inode) SetName(name string) {
	i.Lock()
	i.DriveItem.Name = name
	i.Unlock()
}

// NodeID returns the inodes ID in the filesystem
func (i *Inode) NodeID() uint64 {
	i.RLock()
	defer i.RUnlock()
	return i.nodeID
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
	i.RLock()
	defer i.RUnlock()
	return i.DriveItem.ID
}

// ParentID returns the ID of this item's parent.
func (i *Inode) ParentID() string {
	i.RLock()
	defer i.RUnlock()
	if i.DriveItem.Parent == nil {
		return ""
	}
	return i.DriveItem.Parent.ID
}

// DriveID returns the ID of the drive that this item is stored in
func (i *Inode) DriveID() string {
	i.RLock()
	defer i.RUnlock()
	return i.DriveItem.DriveID()
}

var pathRexp *regexp.Regexp = regexp.MustCompile(`^.+:/?`)

// Path returns an inode's full Path. Used for debugging and logs only as it is
// not guaranteed to be reliable.
func (i *Inode) Path() string {
	// special case when it's the root item
	name := i.Name()
	if i.ParentID() == "" && name == "root" {
		return "/"
	}

	// all paths come prefixed with "/drive/root:"
	i.RLock()
	defer i.RUnlock()
	if i.DriveItem.Parent == nil {
		return name
	}

	path := pathRexp.ReplaceAllString(i.DriveItem.Parent.Path, "/") + "/" + name
	return strings.Replace(path, "//", "/", -1)
}

// HasContent returns whether the file has been populated with data
func (i *Inode) HasContent() bool {
	i.RLock()
	defer i.RUnlock()
	return i.data != nil
}

// HasChanges returns true if the file has local changes that haven't been
// uploaded yet.
func (i *Inode) HasChanges() bool {
	i.RLock()
	defer i.RUnlock()
	return i.hasChanges
}

// HasChildren returns true if the item has more than 0 children
func (i *Inode) HasChildren() bool {
	i.RLock()
	defer i.RUnlock()
	return len(i.children) > 0
}

// makeattr is a convenience function to create a set of filesystem attrs for
// use with syscalls that use or modify attrs.
func (i *Inode) makeAttr() fuse.Attr {
	mtime := i.ModTime()
	return fuse.Attr{
		Ino:   i.NodeID(),
		Size:  i.Size(),
		Nlink: i.NLink(),
		Ctime: mtime,
		Mtime: mtime,
		Atime: mtime,
		Mode:  i.Mode(),
		// whatever user is running the filesystem is the owner
		Owner: fuse.Owner{
			Uid: uint32(os.Getuid()),
			Gid: uint32(os.Getgid()),
		},
	}
}

// IsDir returns if it is a directory (true) or file (false).
func (i *Inode) IsDir() bool {
	// 0 if the dir bit is not set
	return i.Mode()&fuse.S_IFDIR > 0
}

// Mode returns the permissions/mode of the file.
func (i *Inode) Mode() uint32 {
	i.RLock()
	defer i.RUnlock()
	if i.mode == 0 { // only 0 if fetched from Graph API
		if i.DriveItem.IsDir() {
			return fuse.S_IFDIR | 0755
		}
		return fuse.S_IFREG | 0644
	}
	return i.mode
}

// ModTime returns the Unix timestamp of last modification (to get a time.Time
// struct, use time.Unix(int64(d.ModTime()), 0))
func (i *Inode) ModTime() uint64 {
	i.RLock()
	defer i.RUnlock()
	return i.DriveItem.ModTimeUnix()
}

// NLink gives the number of hard links to an inode (or child count if a
// directory)
func (i *Inode) NLink() uint32 {
	if i.IsDir() {
		i.RLock()
		defer i.RUnlock()
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
	i.RLock()
	defer i.RUnlock()
	return i.DriveItem.Size
}

// Octal converts a number to its octal representation in string form.
func Octal(i uint32) string {
	return strconv.FormatUint(uint64(i), 8)
}
