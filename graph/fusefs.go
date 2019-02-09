package graph

import (
	"bytes"
	"encoding/json"
	"log"
	"regexp"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
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

// find where a file's basename + dirname split
func pathSplit(path string) int {
	if path[len(path)-1] == '/' {
		// remove trailing slash (if one exists) for easier regex
		path = path[:len(path)-1]
	}

	//TODO this is really lazy and doesn't account for a nonalphanumeric
	// character in a filename. It should accept anything that isn't an explicit
	// '/' (and ignore escaped '\/'es)
	re := regexp.MustCompile(`\w+$`)
	return re.FindStringIndex(path)[0]
}

// equivalent to the bash basename cmd
func basename(path string) string {
	return path[pathSplit(path):]
}

// equivalent to the bash dirname cmd
func dirname(path string) string {
	return path[:pathSplit(path)]
}

// FuseFs is a memory-backed filesystem for Microsoft Graph
type FuseFs struct {
	pathfs.FileSystem
	Auth     Auth
	items    *ItemCache    // all DriveItems (read: files/folders) are cached
	reqCache *RequestCache // some requests are cached
}

// NewFS initializes a new Graph Filesystem to be used by go-fuse
func NewFS() *FuseFs {
	return &FuseFs{
		FileSystem: pathfs.NewDefaultFileSystem(),
		Auth:       Authenticate(),
		items:      NewItemCache(),
		reqCache:   NewRequestCache(),
	}
}

// GetAttr returns a stat structure for the specified file
func (fs *FuseFs) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	name = "/" + name
	if ignore(name) {
		return nil, fuse.ENOENT
	}
	log.Printf("GetAttr(\"%s\")\n", name)
	item, err := fs.items.Get(name, fs.Auth)
	if err != nil {
		log.Println(err)
		return nil, fuse.ENOENT
	}
	attr := fuse.Attr{}
	status := item.GetAttr(&attr)
	return &attr, status
}

// Chown currently does nothing - it is not a valid option, since fuse is single-user anyways
func (fs *FuseFs) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

// Chmod currently does nothing - no way to change mode yet.
func (fs *FuseFs) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

// OpenDir returns a list of directory entries
func (fs *FuseFs) OpenDir(name string, context *fuse.Context) (c []fuse.DirEntry, code fuse.Status) {
	name = "/" + name
	log.Printf("OpenDir(\"%s\")\n", name)
	children, err := GetChildren(name, fs.Auth, fs.reqCache)
	if err != nil {
		// that directory probably doesn't exist. silly human.
		return nil, fuse.ENOENT
	}
	for _, child := range children {
		entry := fuse.DirEntry{
			Name: child.Name,
			Mode: child.Mode(),
		}
		c = append(c, entry)
	}
	return c, fuse.OK
}

type newFolderPost struct {
	Name   string   `json:"name"`
	Folder struct{} `json:"folder"`
}

// Mkdir creates a directory, mode is ignored
//TODO fix "File exists" case when folder is created, deleted, then created again
func (fs *FuseFs) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	name = "/" + name
	log.Printf("Mkdir(\"%s\")\n", name)

	bytePayload, _ := json.Marshal(newFolderPost{Name: basename(name)})
	resp, err := Post(ChildrenPath(dirname(name)), fs.Auth, bytes.NewReader(bytePayload))
	if err != nil {
		log.Println(string(resp))
		log.Println(err)
		return fuse.EREMOTEIO
	}
	return fuse.OK
}

// Rmdir removes a directory
func (fs *FuseFs) Rmdir(name string, context *fuse.Context) fuse.Status {
	name = "/" + name
	log.Printf("Rmdir(\"%s\")\n", name)
	err := Delete(ResourcePath(name), fs.Auth)
	if err != nil {
		log.Println(err)
		return fuse.EREMOTEIO
	}
	fs.reqCache.Delete(ChildrenPath(dirname(name)))
	fs.items.Delete(name)
	return fuse.OK
}

// Open populates a DriveItem's Data field with actual data
func (fs *FuseFs) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = "/" + name
	log.Printf("Open(\"%s\")\n", name)
	item, err := fs.items.Get(name, fs.Auth)
	if err != nil {
		// doesn't exist or internet is out - either way, no files for you!
		return nil, fuse.ENOENT
	}

	// check for if file has already been populated
	if item.Data == nil {
		// it is unpopulated, grab from api
		log.Println("Fetching remote content for", item.Name)
		body, err := Get("/me/drive/items/"+item.ID+"/content", fs.Auth)
		if err != nil {
			log.Printf("Failed to fetch content for '%s': %s\n", item.ID, err)
			return nil, fuse.EREMOTEIO
		}
		item.Data = &body
		item.File = nodefs.NewDefaultFile()
	}
	return item, fuse.OK
}

// Create a new local file. The server doesn't have this yet.
func (fs *FuseFs) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = "/" + name
	log.Printf("Create(\"%s\")\n", name)

	// fetch details about the new item's parent (need the ID from the remote)
	parentPath := dirname(name)
	parent, err := fs.items.Get(parentPath, fs.Auth)
	if err != nil {
		return nil, fuse.EREMOTEIO
	}

	item := DriveItem{
		Name: basename(name),
		Parent: DriveItemParent{
			ID:   parent.ID,
			Path: parentPath,
		},
	}
	return item, fuse.OK
}
