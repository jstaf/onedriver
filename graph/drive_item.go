package graph

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

// DriveItemParent describes a DriveItem's parent in the Graph API (just another
// DriveItem's ID and its path)
type DriveItemParent struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// DriveItem represents a file or folder fetched from the Graph API
type DriveItem struct {
	nodefs.File
	Data       *[]byte         // empty by default
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Size       uint64          `json:"size"`
	ModifyTime time.Time       `json:"lastModifiedDatetime"` // a string timestamp
	Parent     DriveItemParent `json:"parentReference"`
	Folder     struct {
		ChildCount uint32 `json:"childCount"`
	} `json:"folder,omitempty"`
	FileAPI struct { // renamed to avoid conflict with nodefs.File interface
		Hashes struct {
			Sha1Hash string `json:"sha1Hash"`
		} `json:"hashes"`
	} `json:"file,omitempty"`
}

func (d DriveItem) String() string {
	l := d.Size
	if l > 10 {
		l = 10
	}
	return fmt.Sprintf("DriveItem(%x)", (*d.Data)[:l])
}

// FetchContent fetches a DriveItem's content and initializes the .Data field.
func (d *DriveItem) FetchContent(auth Auth) error {
	body, err := Get("/me/drive/items/"+d.ID+"/content", auth)
	if err != nil {
		return err
	}
	d.Data = &body
	d.File = nodefs.NewDefaultFile()
	return nil
}

// Read from a DriveItem like a file
func (d DriveItem) Read(buf []byte, off int64) (res fuse.ReadResult, code fuse.Status) {
	end := int(off) + int(len(buf))
	if end > len(*d.Data) {
		end = len(*d.Data)
	}
	log.Printf("Read(\"%s\"): %d bytes at offset %d\n", d.Name, int64(end)-off, off)
	return fuse.ReadResultData((*d.Data)[off:end]), fuse.OK
}

// Write to a DriveItem like a file. Note that changes are 100% local until
// Flush() is called.
func (d DriveItem) Write(data []byte, off int64) (uint32, fuse.Status) {
	nWrite := len(data)
	offset := int(off)
	log.Printf("Write(\"%s\"): %d bytes at offset %d\n", d.Name, nWrite, off)

	if offset+nWrite > int(d.Size)-1 {
		// we've exceeded the file size, overwrite via append
		*d.Data = append((*d.Data)[:offset], data...)
	} else {
		// writing inside the current file, overwrite in place
		copy((*d.Data)[offset:], data)
	}
	// probably a better way to do this, but whatever
	d.Size = uint64(len(*d.Data))

	return uint32(nWrite), fuse.OK
}

// Flush is called when a file descriptor is closed, and is responsible for upload
func (d DriveItem) Flush() fuse.Status {
	log.Printf("Flush(\"%s\")\n", d.Name)
	return fuse.OK
}

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
