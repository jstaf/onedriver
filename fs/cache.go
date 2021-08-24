package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

// Filesystem is the actual FUSE filesystem and uses the go analogy of the
// "low-level" FUSE API here:
// https://github.com/libfuse/libfuse/blob/master/include/fuse_lowlevel.h
type Filesystem struct {
	fuse.RawFileSystem

	metadata  sync.Map
	db        *bolt.DB
	auth      *graph.Auth
	root      string // the id of the filesystem's root item
	deltaLink string
	uploads   *UploadManager

	sync.RWMutex
	offline    bool
	lastNodeID uint64
	inodes     []string

	// tracks currently open directories
	opendirsM sync.RWMutex
	opendirs  map[uint64][]*Inode
}

// boltdb buckets
var (
	bucketContent  = []byte("content")
	bucketMetadata = []byte("metadata")
	bucketDelta    = []byte("delta")
)

// NewFilesystem creates a new filesystem
func NewFilesystem(auth *graph.Auth, dbpath string) *Filesystem {
	db, err := bolt.Open(dbpath, 0600, &bolt.Options{Timeout: time.Second * 5})
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Fatal("Could not open DB")
	}
	db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists(bucketContent)
		tx.CreateBucketIfNotExists(bucketMetadata)
		tx.CreateBucketIfNotExists(bucketDelta)
		return nil
	})
	fs := &Filesystem{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
		auth:          auth,
		db:            db,
		opendirs:      make(map[uint64][]*Inode),
	}

	rootItem, err := graph.GetItem("root", auth)
	root := NewInodeDriveItem(rootItem)
	if err != nil {
		if graph.IsOffline(err) {
			// no network, load from db if possible and go to read-only state
			fs.Lock()
			fs.offline = true
			fs.Unlock()
			if root = fs.GetID("root"); root == nil {
				log.Fatal("We are offline and could not fetch the filesystem root item from disk.")
			}
			// when offline, we load the cache deltaLink from disk
			fs.db.View(func(tx *bolt.Tx) error {
				if link := tx.Bucket(bucketDelta).Get([]byte("deltaLink")); link != nil {
					fs.deltaLink = string(link)
				} else {
					// Only reached if a previous online session never survived
					// long enough to save its delta link. We explicitly disallow these
					// types of startups as it's possible for things to get out of sync
					// this way.
					log.Fatal("Cannot perform an offline startup without a valid delta " +
						"link from a previous session.")
				}
				return nil
			})
		} else {
			log.WithFields(log.Fields{
				"err": err,
			}).Fatal("Could not fetch root item of filesystem!")
		}
	}
	// root inode is inode 1
	fs.root = root.ID()
	fs.InsertID(fs.root, root)

	fs.uploads = NewUploadManager(2*time.Second, db, fs, auth)

	if !fs.IsOffline() {
		// .Trash-UID is used by "gio trash" for user trash, create it if it
		// does not exist
		trash := fmt.Sprintf(".Trash-%d", os.Getuid())
		if child, _ := fs.GetChild(fs.root, trash, auth); child == nil {
			item, err := graph.Mkdir(trash, fs.root, auth)
			if err != nil {
				log.WithField("err", err).Error("Could not create trash folder. " +
					"Trashing items through the file browser may result in errors.")
			} else {
				fs.InsertID(item.ID, NewInodeDriveItem(item))
			}
		}

		// using token=latest because we don't care about existing items - they'll
		// be downloaded on-demand by the cache
		fs.deltaLink = "/me/drive/root/delta?token=latest"
	}

	// deltaloop is started manually
	return fs
}

// IsOffline returns whether or not the cache thinks its offline.
func (f *Filesystem) IsOffline() bool {
	f.RLock()
	defer f.RUnlock()
	return f.offline
}

// TranslateID returns the DriveItemID for a given NodeID
func (f *Filesystem) TranslateID(nodeID uint64) string {
	f.RLock()
	defer f.RUnlock()
	if nodeID > f.lastNodeID || nodeID == 0 {
		return ""
	}
	return f.inodes[nodeID-1]
}

// GetNodeID fetches the inode for a particular inode ID.
func (f *Filesystem) GetNodeID(nodeID uint64) *Inode {
	id := f.TranslateID(nodeID)
	if id == "" {
		return nil
	}
	return f.GetID(id)
}

// InsertNodeID assigns a numeric inode ID used by the kernel if one is not
// already assigned.
func (f *Filesystem) InsertNodeID(inode *Inode) uint64 {
	nodeID := inode.NodeID()
	if nodeID == 0 {
		f.Lock()
		f.lastNodeID++

		inode.mutex.Lock()
		f.inodes = append(f.inodes, inode.DriveItem.ID)
		nodeID = f.lastNodeID
		inode.nodeID = nodeID
		inode.mutex.Unlock()

		f.Unlock()
	}
	return nodeID
}

// GetID gets an inode from the cache by ID. No API fetching is performed.
// Result is nil if no inode is found.
func (f *Filesystem) GetID(id string) *Inode {
	entry, exists := f.metadata.Load(id)
	if !exists {
		// we allow fetching from disk as a fallback while offline (and it's also
		// necessary while transitioning from offline->online)
		var found *Inode
		f.db.View(func(tx *bolt.Tx) error {
			data := tx.Bucket(bucketMetadata).Get([]byte(id))
			var err error
			if data != nil {
				found, err = NewInodeJSON(data)
			}
			return err
		})
		if found != nil {
			f.InsertNodeID(found)
			f.metadata.Store(id, found) // move to memory for next time
		}
		return found
	}
	return entry.(*Inode)
}

// InsertID inserts a single item into the filesystem by ID and sets its parent
// using the Inode.Parent.ID, if set. Must be called after DeleteID, if being
// used to rename/move an item. This is the main way new Inodes are added to the
// filesystem. Returns the Inode's numeric NodeID.
func (f *Filesystem) InsertID(id string, inode *Inode) uint64 {
	// make sure the item knows about the cache itself, then insert
	f.metadata.Store(id, inode)
	nodeID := f.InsertNodeID(inode)

	if id != inode.ID() {
		// we update the inode IDs here in case they do not match/changed
		inode.mutex.Lock()
		inode.DriveItem.ID = id
		inode.mutex.Unlock()

		f.Lock()
		if nodeID < f.lastNodeID {
			f.inodes[nodeID-1] = id
		} else {
			log.WithFields(log.Fields{
				"nodeID":     nodeID,
				"lastNodeID": f.lastNodeID,
			}).Error("NodeID exceeded maximum node ID! Ignoring ID change.")
		}
		f.Unlock()
	}

	parentID := inode.ParentID()
	if parentID == "" {
		// root item, or parent not set
		return nodeID
	}
	parent := f.GetID(parentID)
	if parent == nil {
		log.WithFields(log.Fields{
			"parentID":  parentID,
			"childID":   id,
			"childName": inode.Name(),
		}).Error("Parent item could not be found when setting parent.")
		return nodeID
	}

	// check if the item has already been added to the parent
	// Lock order is super key here, must go parent->child or the deadlock
	// detector screams at us.
	parent.mutex.Lock()
	defer parent.mutex.Unlock()
	for _, child := range parent.children {
		if child == id {
			// exit early, child cannot be added twice
			return nodeID
		}
	}

	// add to parent
	if inode.IsDir() {
		parent.subdir++
	}
	parent.children = append(parent.children, id)

	return nodeID
}

// InsertChild adds an item as a child of a specified parent ID.
func (f *Filesystem) InsertChild(parentID string, child *Inode) uint64 {
	child.mutex.Lock()
	// should already be set, just double-checking here.
	child.DriveItem.Parent.ID = parentID
	id := child.DriveItem.ID
	child.mutex.Unlock()
	return f.InsertID(id, child)
}

// DeleteID deletes an item from the cache, and removes it from its parent. Must
// be called before InsertID if being used to rename/move an item.
func (f *Filesystem) DeleteID(id string) {
	if inode := f.GetID(id); inode != nil {
		parent := f.GetID(inode.ParentID())
		parent.mutex.Lock()
		for i, childID := range parent.children {
			if childID == id {
				parent.children = append(parent.children[:i], parent.children[i+1:]...)
				if inode.IsDir() {
					parent.subdir--
				}
				break
			}
		}
		parent.mutex.Unlock()
	}
	f.metadata.Delete(id)
	f.uploads.CancelUpload(id)
}

// GetChild fetches a named child of an item. Wraps GetChildrenID.
func (f *Filesystem) GetChild(id string, name string, auth *graph.Auth) (*Inode, error) {
	children, err := f.GetChildrenID(id, auth)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if strings.EqualFold(child.Name(), name) {
			return child, nil
		}
	}
	return nil, errors.New("child does not exist")
}

// GetChildrenID grabs all DriveItems that are the children of the given ID. If
// items are not found, they are fetched.
func (f *Filesystem) GetChildrenID(id string, auth *graph.Auth) (map[string]*Inode, error) {
	// fetch item and catch common errors
	inode := f.GetID(id)
	children := make(map[string]*Inode)
	if inode == nil {
		log.WithFields(log.Fields{
			"id": id,
		}).Error("Inode not found in cache")
		return children, errors.New(id + " not found in cache")
	} else if !inode.IsDir() {
		// Normal files are treated as empty folders. This only gets called if
		// we messed up and tried to get the children of a plain-old file.
		log.WithFields(log.Fields{
			"id":   id,
			"path": inode.Path(),
		}).Warn("Attepted to get children of ordinary file")
		return children, nil
	}

	// If item.children is not nil, it means we have the item's children
	// already and can fetch them directly from the cache
	inode.mutex.RLock()
	if inode.children != nil {
		// can potentially have out-of-date child metadata if started offline, but since
		// changes are disallowed while offline, the children will be back in sync after
		// the first successful delta fetch (which also brings the fs back online)
		for _, childID := range inode.children {
			child := f.GetID(childID)
			if child == nil {
				// will be nil if deleted or never existed
				continue
			}
			children[strings.ToLower(child.Name())] = child
		}
		inode.mutex.RUnlock()
		return children, nil
	}
	inode.mutex.RUnlock()

	// We haven't fetched the children for this item yet, get them from the server.
	fetched, err := graph.GetItemChildren(id, auth)
	if err != nil {
		if graph.IsOffline(err) {
			log.WithFields(log.Fields{
				"id": id,
			}).Warn("We are offline, and no children found in cache. Pretending there are no children.")
			return children, nil
		}
		// something else happened besides being offline
		return nil, err
	}

	inode.mutex.Lock()
	inode.children = make([]string, 0)
	for _, item := range fetched {
		// we will always have an id after fetching from the server
		child := NewInodeDriveItem(item)
		f.InsertNodeID(child)
		f.metadata.Store(child.DriveItem.ID, child)

		// store in result map
		children[strings.ToLower(child.Name())] = child

		// store id in parent item and increment parents subdirectory count
		inode.children = append(inode.children, child.DriveItem.ID)
		if child.IsDir() {
			inode.subdir++
		}
	}
	inode.mutex.Unlock()

	return children, nil
}

// GetChildrenPath grabs all DriveItems that are the children of the resource at
// the path. If items are not found, they are fetched.
func (f *Filesystem) GetChildrenPath(path string, auth *graph.Auth) (map[string]*Inode, error) {
	inode, err := f.GetPath(path, auth)
	if err != nil {
		return make(map[string]*Inode), err
	}
	return f.GetChildrenID(inode.ID(), auth)
}

// GetPath fetches a given DriveItem in the cache, if any items along the way are
// not found, they are fetched.
func (f *Filesystem) GetPath(path string, auth *graph.Auth) (*Inode, error) {
	lastID := f.root
	if path == "/" {
		return f.GetID(lastID), nil
	}

	// from the root directory, traverse the chain of items till we reach our
	// target ID.
	path = strings.TrimSuffix(strings.ToLower(path), "/")
	split := strings.Split(path, "/")[1:] //omit leading "/"
	var inode *Inode
	for i := 0; i < len(split); i++ {
		// fetches children
		children, err := f.GetChildrenID(lastID, auth)
		if err != nil {
			return nil, err
		}

		var exists bool // if we use ":=", item is shadowed
		inode, exists = children[split[i]]
		if !exists {
			// the item still doesn't exist after fetching from server. it
			// doesn't exist
			return nil, errors.New(strings.Join(split[:i+1], "/") +
				" does not exist on server or in local cache")
		}
		lastID = inode.ID()
	}
	return inode, nil
}

// DeletePath an item from the cache by path. Must be called before Insert if
// being used to move/rename an item.
func (f *Filesystem) DeletePath(key string) {
	inode, _ := f.GetPath(strings.ToLower(key), nil)
	if inode != nil {
		f.DeleteID(inode.ID())
	}
}

// InsertPath lets us manually insert an item to the cache (like if it was
// created locally). Overwrites a cached item if present. Must be called after
// delete if being used to move/rename an item.
func (f *Filesystem) InsertPath(key string, auth *graph.Auth, inode *Inode) (uint64, error) {
	key = strings.ToLower(key)

	// set the item.Parent.ID properly if the item hasn't been in the cache
	// before or is being moved.
	parent, err := f.GetPath(filepath.Dir(key), auth)
	if err != nil {
		return 0, err
	} else if parent == nil {
		const errMsg string = "parent of key was nil"
		log.WithFields(log.Fields{
			"key":  key,
			"path": inode.Path(),
		}).Error(errMsg)
		return 0, errors.New(errMsg)
	}

	// Coded this way to make sure locks are in the same order for the deadlock
	// detector (lock ordering needs to be the same as InsertID: Parent->Child).
	parentID := parent.ID()
	inode.mutex.Lock()
	inode.DriveItem.Parent.ID = parentID
	inode.mutex.Unlock()

	return f.InsertID(inode.ID(), inode), nil
}

// MoveID moves an item to a new ID name. Also responsible for handling the
// actual overwrite of the item's IDInternal field
func (f *Filesystem) MoveID(oldID string, newID string) error {
	inode := f.GetID(oldID)
	if inode == nil {
		// It may have already been renamed. This is not an error. We assume
		// that IDs will never collide. Re-perform the op if this is the case.
		if inode = f.GetID(newID); inode == nil {
			// nope, it just doesn't exist
			return errors.New("Could not get item: " + oldID)
		}
	}

	// need to rename the child under the parent
	parent := f.GetID(inode.ParentID())
	parent.mutex.Lock()
	for i, child := range parent.children {
		if child == oldID {
			parent.children[i] = newID
			break
		}
	}
	parent.mutex.Unlock()

	inode.mutex.Lock()
	inode.DriveItem.ID = newID
	inode.mutex.Unlock()

	// now actually perform the metadata+content move
	f.DeleteID(oldID)
	f.InsertID(newID, inode)
	if inode.IsDir() {
		return nil
	}
	return f.MoveContent(oldID, newID)
}

// MovePath moves an item to a new position.
func (f *Filesystem) MovePath(oldParent, newParent, oldName, newName string, auth *graph.Auth) error {
	inode, err := f.GetChild(oldParent, oldName, auth)
	if err != nil {
		return err
	}

	id := inode.ID()
	f.DeleteID(id)

	// this is the actual move op
	inode.SetName(newName)
	parent := f.GetID(newParent)
	inode.Parent.ID = parent.DriveItem.ID
	f.InsertID(id, inode)
	return nil
}

// GetContent reads a file's content from disk.
func (f *Filesystem) GetContent(id string) []byte {
	var content []byte // nil
	f.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketContent)
		if tmp := b.Get([]byte(id)); tmp != nil {
			content = make([]byte, len(tmp))
			copy(content, tmp)
		}
		return nil
	})
	return content
}

// InsertContent writes file content to disk.
func (f *Filesystem) InsertContent(id string, content []byte) error {
	return f.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketContent)
		return b.Put([]byte(id), content)
	})
}

// DeleteContent deletes content from disk.
func (f *Filesystem) DeleteContent(id string) error {
	return f.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketContent)
		return b.Delete([]byte(id))
	})
}

// MoveContent moves content from one ID to another
func (f *Filesystem) MoveContent(oldID string, newID string) error {
	return f.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketContent)
		content := b.Get([]byte(oldID))
		if content == nil {
			return errors.New("Content not found for ID: " + oldID)
		}
		b.Put([]byte(newID), content)
		b.Delete([]byte(oldID))
		return nil
	})
}

// SerializeAll dumps all inode metadata currently in the cache to disk. This
// metadata is only used later if an item could not be found in memory AND the
// cache is offline. Old metadata is not removed, only overwritten (to avoid an
// offline session from wiping all metadata on a subsequent serialization).
func (f *Filesystem) SerializeAll() {
	log.Debug("Serializing cache metadata to disk.")
	f.metadata.Range(func(key interface{}, value interface{}) bool {
		f.db.Batch(func(tx *bolt.Tx) error {
			id := fmt.Sprint(key)
			contents := value.(*Inode).AsJSON()
			b := tx.Bucket(bucketMetadata)
			b.Put([]byte(id), contents)
			if id == f.root {
				// root item must be updated manually (since there's actually
				// two copies)
				b.Put([]byte("root"), contents)
			}
			return nil
		})
		return true
	})
}
