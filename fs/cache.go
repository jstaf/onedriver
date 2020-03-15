package fs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "github.com/etcd-io/bbolt"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/jstaf/onedriver/fs/graph"
	log "github.com/sirupsen/logrus"
)

// Cache caches Inodes for a filesystem. This cache never expires so that local
// changes can persist. Should be created using the NewCache() constructor.
type Cache struct {
	metadata  sync.Map
	db        *bolt.DB
	root      string // the id of the filesystem's root item
	deltaLink string
	uploads   *UploadManager

	sync.RWMutex
	auth      *graph.Auth
	driveType string // personal | business
	offline   bool
}

// boltdb buckets
var (
	CONTENT  = []byte("content")
	METADATA = []byte("metadata")
	DELTA    = []byte("delta")
)

// NewCache creates a new Cache
func NewCache(auth *graph.Auth, dbpath string) *Cache {
	db, err := bolt.Open(dbpath, 0600, &bolt.Options{Timeout: time.Second * 5})
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Fatal("Could not open DB")
	}
	db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists(CONTENT)
		tx.CreateBucketIfNotExists(METADATA)
		tx.CreateBucketIfNotExists(DELTA)
		return nil
	})
	cache := &Cache{
		auth: auth,
		db:   db,
	}

	rootItem, err := graph.GetItem("root", auth)
	root := NewInodeDriveItem(rootItem)
	if err != nil {
		if graph.IsOffline(err) {
			// no network, load from db if possible and go to read-only state
			cache.Lock()
			cache.offline = true
			cache.Unlock()
			if root = cache.GetID("root"); root == nil {
				log.Fatal("We are offline and could not fetch the filesystem root item from disk.")
			}
			// when offline, we load the cache deltaLink from disk
			cache.db.View(func(tx *bolt.Tx) error {
				if link := tx.Bucket(DELTA).Get([]byte("deltaLink")); link != nil {
					cache.deltaLink = string(link)
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
	root.cache = cache
	cache.root = root.ID()
	cache.InsertID(cache.root, root)

	cache.uploads = NewUploadManager(2*time.Second, auth)

	if !cache.IsOffline() {
		// .Trash-UID is used by "gio trash" for user trash, create it if it
		// does not exist
		trash := fmt.Sprintf(".Trash-%d", os.Getuid())
		if child, _ := cache.GetChild(cache.root, trash, auth); child == nil {
			item, err := graph.Mkdir(trash, cache.root, auth)
			if err != nil {
				log.WithField("err", err).Error("Could not create trash folder. " +
					"Trashing items through the file browser may result in errors.")
			} else {
				cache.InsertID(item.ID, NewInodeDriveItem(item))
			}
		}

		// using token=latest because we don't care about existing items - they'll
		// be downloaded on-demand by the cache
		cache.deltaLink = "/me/drive/root/delta?token=latest"
	}

	// deltaloop is started manually
	return cache
}

// GetAuth returns the current auth
func (c *Cache) GetAuth() *graph.Auth {
	c.RLock()
	defer c.RUnlock()
	return c.auth
}

// IsOffline returns whether or not the cache thinks its offline.
func (c *Cache) IsOffline() bool {
	c.RLock()
	defer c.RUnlock()
	return c.offline
}

// DriveType lazily fetches the OneDrive drivetype
func (c *Cache) DriveType() string {
	c.RLock()
	driveType := c.driveType
	c.RUnlock()

	if driveType == "" {
		drive, err := graph.GetDrive(c.GetAuth())
		if err == nil {
			c.Lock()
			c.driveType = drive.DriveType
			c.Unlock()
			return drive.DriveType
		}
		log.Error("Drivetype was empty and could not be fetched!")
	}
	return driveType
}

func leadingSlash(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// InodePath calculates an inode's path to the filesystem root
func (c *Cache) InodePath(fuseInode *fs.Inode) string {
	root, _ := c.GetPath("/", nil)
	return leadingSlash(fuseInode.Path(root.EmbeddedInode()))
}

// GetID gets an inode from the cache by ID. No API fetching is performed.
// Result is nil if no inode is found.
func (c *Cache) GetID(id string) *Inode {
	entry, exists := c.metadata.Load(id)
	if !exists {
		// we allow fetching from disk as a fallback while offline (and it's also
		// necessary while transitioning from offline->online)
		var found *Inode
		c.db.View(func(tx *bolt.Tx) error {
			data := tx.Bucket(METADATA).Get([]byte(id))
			var err error
			if data != nil {
				found, err = NewInodeJSON(data)
			}
			return err
		})
		if found != nil {
			found.cache = c
			c.metadata.Store(id, found) // move to memory for next time
		}
		return found
	}
	return entry.(*Inode)
}

// InsertID inserts a single item into the cache by ID and sets its parent using
// the Inode.Parent.ID, if set. Must be called after DeleteID, if being used to
// rename/move an item.
func (c *Cache) InsertID(id string, inode *Inode) {
	// make sure the item knows about the cache itself, then insert
	inode.mutex.Lock()
	inode.cache = c
	inode.mutex.Unlock()
	c.metadata.Store(id, inode)

	parentID := inode.ParentID()
	if parentID == "" {
		// root item, or parent not set
		return
	}
	parent := c.GetID(parentID)
	if parent == nil {
		log.WithFields(log.Fields{
			"parentID":  parentID,
			"childID":   id,
			"childName": inode.Name(),
		}).Error("Parent item could not be found when setting parent.")
		return
	}

	// check if the item has already been added to the parent
	// Lock order is super key here, must go parent->child or the deadlock
	// detector screams at us.
	parent.mutex.Lock()
	defer parent.mutex.Unlock()
	for _, child := range parent.children {
		if child == id {
			// exit early, child cannot be added twice
			return
		}
	}

	// add to parent
	if inode.IsDir() {
		parent.subdir++
	}
	parent.children = append(parent.children, inode.ID())
}

// InsertChild adds an item as a child of a specified parent ID.
func (c *Cache) InsertChild(parentID string, child *Inode) {
	child.mutex.Lock()
	// should already be set, just double-checking here.
	child.DriveItem.Parent.ID = parentID
	id := child.DriveItem.ID
	child.mutex.Unlock()
	c.InsertID(id, child)
}

// DeleteID deletes an item from the cache, and removes it from its parent. Must
// be called before InsertID if being used to rename/move an item.
func (c *Cache) DeleteID(id string) {
	if inode := c.GetID(id); inode != nil {
		parent := c.GetID(inode.ParentID())
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
	c.metadata.Delete(id)
}

// only used for parsing
type driveChildren struct {
	Children []*Inode `json:"value"`
}

// GetChild fetches a named child of an item. Wraps GetChildrenID.
func (c *Cache) GetChild(id string, name string, auth *graph.Auth) (*Inode, error) {
	children, err := c.GetChildrenID(id, auth)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if strings.ToLower(child.Name()) == strings.ToLower(name) {
			return child, nil
		}
	}
	return nil, errors.New("Child does not exist")
}

// GetChildrenID grabs all DriveItems that are the children of the given ID. If
// items are not found, they are fetched.
func (c *Cache) GetChildrenID(id string, auth *graph.Auth) (map[string]*Inode, error) {
	// fetch item and catch common errors
	inode := c.GetID(id)
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
			child := c.GetID(childID)
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

	// We haven't fetched the children for this item yet, get them from the
	// server.
	body, err := graph.Get(graph.ChildrenPathID(id), auth)
	if err != nil {
		if graph.IsOffline(err) {
			log.WithFields(log.Fields{
				"id": id,
			}).Warn("We are offline, and no children found in cache. Pretending there are no children.")
			return children, nil
		}
		// something else happened besides being offline
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Error while fetching children.")
		return nil, err
	}
	var fetched driveChildren
	json.Unmarshal(body, &fetched)

	inode.mutex.Lock()
	inode.children = make([]string, 0)
	for _, child := range fetched.Children {
		// we will always have an id after fetching from the server
		child.cache = c
		c.metadata.Store(child.DriveItem.ID, child)

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
func (c *Cache) GetChildrenPath(path string, auth *graph.Auth) (map[string]*Inode, error) {
	inode, err := c.GetPath(path, auth)
	if err != nil {
		return make(map[string]*Inode), err
	}
	return c.GetChildrenID(inode.ID(), auth)
}

// GetPath fetches a given DriveItem in the cache, if any items along the way are
// not found, they are fetched.
func (c *Cache) GetPath(path string, auth *graph.Auth) (*Inode, error) {
	lastID := c.root
	if path == "/" {
		return c.GetID(lastID), nil
	}

	// from the root directory, traverse the chain of items till we reach our
	// target ID.
	path = strings.TrimSuffix(strings.ToLower(path), "/")
	split := strings.Split(path, "/")[1:] //omit leading "/"
	var inode *Inode
	for i := 0; i < len(split); i++ {
		// fetches children
		children, err := c.GetChildrenID(lastID, auth)
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
func (c *Cache) DeletePath(key string) {
	inode, _ := c.GetPath(strings.ToLower(key), nil)
	if inode != nil {
		c.DeleteID(inode.ID())
	}
}

// InsertPath lets us manually insert an item to the cache (like if it was
// created locally). Overwrites a cached item if present. Must be called after
// delete if being used to move/rename an item.
func (c *Cache) InsertPath(key string, auth *graph.Auth, inode *Inode) error {
	key = strings.ToLower(key)

	// set the item.Parent.ID properly if the item hasn't been in the cache
	// before or is being moved.
	parent, err := c.GetPath(filepath.Dir(key), auth)
	if err != nil {
		return err
	} else if parent == nil {
		const errMsg string = "Parent of key was nil! Did we accidentally use an ID for the key?"
		log.WithFields(log.Fields{
			"key":  key,
			"path": inode.Path(),
		}).Error(errMsg)
		return errors.New(errMsg)
	}

	// Coded this way to make sure locks are in the same order for the deadlock
	// detector (lock ordering needs to be the same as InsertID: Parent->Child).
	parentID := parent.ID()
	inode.mutex.Lock()
	inode.DriveItem.Parent.ID = parentID
	inode.mutex.Unlock()

	c.InsertID(inode.ID(), inode)
	return nil
}

// MoveID moves an item to a new ID name. Also responsible for handling the
// actual overwrite of the item's IDInternal field
func (c *Cache) MoveID(oldID string, newID string) error {
	inode := c.GetID(oldID)
	if inode == nil {
		// It may have already been renamed. This is not an error. We assume
		// that IDs will never collide. Re-perform the op if this is the case.
		if inode = c.GetID(newID); inode == nil {
			// nope, it just doesn't exist
			return errors.New("Could not get item: " + oldID)
		}
	}

	// need to rename the child under the parent
	parent := c.GetID(inode.ParentID())
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
	c.DeleteID(oldID)
	c.InsertID(newID, inode)
	c.MoveContent(oldID, newID)
	return nil
}

// MovePath an item to a new position
func (c *Cache) MovePath(oldPath string, newPath string, auth *graph.Auth) error {
	inode, err := c.GetPath(oldPath, auth)
	if err != nil {
		return err
	}

	c.DeletePath(oldPath)
	if newBase := filepath.Base(newPath); filepath.Base(oldPath) != newBase {
		inode.SetName(newBase)
	}
	if err := c.InsertPath(newPath, auth, inode); err != nil {
		// insert failed, reinsert in old location
		inode.SetName(filepath.Base(oldPath))
		c.InsertPath(oldPath, auth, inode)
		return err
	}
	return nil
}

// GetContent reads a file's content from disk.
func (c *Cache) GetContent(id string) []byte {
	var content []byte // nil
	c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(CONTENT)
		if tmp := b.Get([]byte(id)); tmp != nil {
			content = make([]byte, len(tmp))
			copy(content, tmp)
		}
		return nil
	})
	return content
}

// InsertContent writes file content to disk.
func (c *Cache) InsertContent(id string, content []byte) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(CONTENT)
		return b.Put([]byte(id), content)
	})
}

// DeleteContent deletes content from disk.
func (c *Cache) DeleteContent(id string) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(CONTENT)
		return b.Delete([]byte(id))
	})
}

// MoveContent moves content from one ID to another
func (c *Cache) MoveContent(oldID string, newID string) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(CONTENT)
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
func (c *Cache) SerializeAll() {
	log.Info("Serializing cache metadata to disk.")
	c.metadata.Range(func(key interface{}, value interface{}) bool {
		c.db.Batch(func(tx *bolt.Tx) error {
			id := fmt.Sprint(key)
			contents := value.(*Inode).AsJSON()
			b := tx.Bucket(METADATA)
			b.Put([]byte(id), contents)
			if id == c.root {
				// root item must be updated manually (since there's actually
				// two copies)
				b.Put([]byte("root"), contents)
			}
			return nil
		})
		return true
	})
}
