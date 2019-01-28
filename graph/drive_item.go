package graph

import (
	"fmt"
	"os"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

// DriveItem represents a file or folder fetched from the Graph API
type DriveItem struct {
	nodefs.File
	data       []byte
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Size       uint64    `json:"size"`
	ModifyTime time.Time `json:"lastModifiedDatetime"` // a string timestamp
	Parent     struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	} `json:"parentReference"`
	Folder struct {
		ChildCount uint32 `json:"childCount"`
	} `json:"folder,omitempty"`
	FileAPI struct { // renamed to avoid conflict with nodefs.File interface
		Hashes struct {
			Sha1Hash string `json:"sha1Hash"`
		} `json:"hashes"`
	} `json:"file,omitempty"`
}

// NewDriveItem creates a new DriveItem
func NewDriveItem(data []byte) DriveItem {
	item := new(DriveItem)
	item.data = data
	item.Size = uint64(len(data))
	item.File = nodefs.NewDefaultFile()
	return *item
}

func (d DriveItem) String() string {
	l := d.Size
	if l > 10 {
		l = 10
	}
	return fmt.Sprintf("DriveItem(%x)", d.data[:l])
}

// Read from a DriveItem like a file
func (d DriveItem) Read(buf []byte, off int64) (res fuse.ReadResult, code fuse.Status) {
	end := int(off) + int(len(buf))
	if end > len(d.data) {
		end = len(d.data)
	}
	return fuse.ReadResultData(d.data[off:end]), fuse.OK
}

/*
// Write to a DriveItem like a file
func (d DriveItem) Write(data []byte, off int64) (uint32, fuse.Status) {
	n := len(data)
	return uint32(n), fuse.OK
}
*/

// GetAttr returns a the DriveItem as a UNIX stat
func (d DriveItem) GetAttr(out *fuse.Attr) fuse.Status {
	out.Size = d.FakeSize()
	out.Atime = d.MTime()
	out.Mtime = d.MTime()
	out.Ctime = d.MTime()
	out.Mode = d.Mode()
	out.Owner = fuse.Owner{
		Uid: uint32(os.Getuid()),
		Gid: uint32(os.Getgid()),
	}
	return fuse.OK
}

// IsDir returns if it is a directory (true) or file (false).
func (d DriveItem) IsDir() bool {
	return d.FileAPI.Hashes.Sha1Hash == ""
}

// Mode returns the permissions/mode of the file.
func (d DriveItem) Mode() uint32 {
	//TODO change when filesystem is writeable
	if d.IsDir() {
		return fuse.S_IFDIR | 0755 // bitwise op: dir + rwx
	}
	return fuse.S_IFREG | 0644 // bitwise op: file + rw-
}

// MTime returns the Unix timestamp of last modification
func (d DriveItem) MTime() uint64 {
	return uint64(d.ModifyTime.Unix())
}

// NLink gives the number of hard links to an inode (or child count if a
// directory)
func (d DriveItem) NLink() uint32 {
	if d.IsDir() {
		return d.Folder.ChildCount
	}
	return 1
}

// FakeSize pretends that folders are 4096 bytes, even though they're 0 (since
// they actually don't exist).
func (d DriveItem) FakeSize() uint64 {
	if d.IsDir() {
		return 4096
	}
	return d.Size
}
