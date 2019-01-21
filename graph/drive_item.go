package graph

import (
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

// DriveItem represents a drive item's fetched from the Graph API
type DriveItem struct {
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
	File struct {
		Hashes struct {
			Sha1Hash string `json:"sha1Hash"`
		} `json:"hashes"`
	} `json:"file,omitempty"`
}

// IsDir returns if it is a directory (true) or file (false).
func (d DriveItem) IsDir() bool {
	return d.File.Hashes.Sha1Hash == ""
}

// Mode returns the permissions/mode of the file.
func (d DriveItem) Mode() uint32 {
	//TODO change when filesystem is writeable
	if d.IsDir() {
		return fuse.S_IFDIR | 0555 // bitwise op: dir + r-x
	}
	return fuse.S_IFREG | 0444 // bitwise op: file + r--
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
