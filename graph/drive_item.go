package graph

import (
	"encoding/json"
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
	Item *DriveItem
}

// DriveItem represents a file or folder fetched from the Graph API
type DriveItem struct {
	nodefs.File
	Data       *[]byte         // empty by default
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Size       *uint64         `json:"size"` // must be a pointer or cannot be modified during write
	ModifyTime time.Time       `json:"lastModifiedDatetime"`
	mode       uint32          // do not set manually
	Parent     DriveItemParent `json:"parentReference"`
	Children   []*DriveItem
	Folder     struct {
		ChildCount uint32 `json:"childCount"`
	} `json:"folder,omitempty"`
	FileAPI struct { // renamed to avoid conflict with nodefs.File interface
		MimeType string `json:"mimeType"`
		Hashes   struct {
			Sha1Hash     string `json:"sha1Hash"`
			QuickXorHash string `json:"quickXorHash"`
		} `json:"hashes"`
	} `json:"file,omitempty"`
}

// NewDriveItem initializes a new DriveItem
func NewDriveItem(name string, mode uint32, parent *DriveItem) *DriveItem {
	var empty []byte
	var children []*DriveItem
	var size uint64
	return &DriveItem{
		File: nodefs.NewDefaultFile(),
		Name: name,
		Parent: DriveItemParent{
			ID:   parent.ID,
			Path: parent.Parent.Path + "/" + parent.Name,
			Item: parent,
		},
		Children:   children,
		Data:       &empty,
		Size:       &size,
		ModifyTime: time.Now(),
		mode:       mode,
	}
}

func (d DriveItem) String() string {
	l := *d.Size
	if l > 10 {
		l = 10
	}
	return fmt.Sprintf("DriveItem(%x)", (*d.Data)[:l])
}

// only used for parsing
type driveChildren struct {
	Children []*DriveItem `json:"value"`
}

// GetChildren fetches all DriveItems that are children of resource at path
func (d *DriveItem) GetChildren(path string, auth Auth) ([]*DriveItem, error) {
	if !d.IsDir() || len(d.Children) > 0 {
		return d.Children, nil
	}

	body, err := Get(ChildrenPath(path), auth)
	var children driveChildren
	if err != nil {
		return children.Children, err
	}
	json.Unmarshal(body, &children)
	d.Children = children.Children

	for _, child := range d.Children {
		child.Parent.Item = d
	}

	return children.Children, nil
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

	if offset+nWrite > int(*d.Size)-1 {
		// we've exceeded the file size, overwrite via append
		*d.Data = append((*d.Data)[:offset], data...)
	} else {
		// writing inside the current file, overwrite in place
		copy((*d.Data)[offset:], data)
	}
	// probably a better way to do this, but whatever
	*d.Size = uint64(len(*d.Data))

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
	// following statement returns 0 if the dir bit is not set
	return d.Mode()&fuse.S_IFDIR > 0
}

// Mode returns the permissions/mode of the file. Lazily initializes the
// underlying mode field.
func (d *DriveItem) Mode() uint32 {
	if d.mode == 0 { // only 0 if fetched from Graph API
		if d.FileAPI.MimeType == "" { // blank if a folder
			d.mode = fuse.S_IFDIR | 0755
		} else {
			d.mode = fuse.S_IFREG | 0644
		}
	}
	return d.mode
}

// MTime returns the Unix timestamp of last modification
func (d DriveItem) MTime() uint64 {
	return uint64(d.ModifyTime.Unix())
}

// Utimens sets the access/modify times of a file
func (d *DriveItem) Utimens(atime *time.Time, mtime *time.Time) fuse.Status {
	d.ModifyTime = *mtime
	return fuse.OK
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
	return *d.Size
}
