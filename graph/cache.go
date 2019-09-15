package graph

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "github.com/etcd-io/bbolt"
	"github.com/hanwen/go-fuse/v2/fs"
	mu "github.com/sasha-s/go-deadlock"
	log "github.com/sirupsen/logrus"
)

// Cache caches DriveItems for a filesystem. This cache never expires so
// that local changes can persist. Should be created using the NewCache()
// constructor.
type Cache struct {
	metadata  sync.Map
	db        *bolt.DB
	auth      *Auth
	mutex     *mu.RWMutex
	root      string // the id of the filesystem's root item
	deltaLink string
}

// NewFS is a wrapper around NewCache
//TODO refactor this out
func NewFS(dbpath string) *DriveItem {
	auth := Authenticate()
	cache := NewCache(auth, dbpath)
	root, _ := cache.GetPath("/", auth)
	return root
}

// NewCache creates a new Cache
func NewCache(auth *Auth, dbpath string) *Cache {
	db, err := bolt.Open(dbpath, 0600, &bolt.Options{Timeout: time.Second * 5})
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Fatal("Could not open DB")
	}
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("content"))
		return err
	})
	cache := &Cache{
		auth:  auth,
		db:    db,
		mutex: &mu.RWMutex{},
	}

	root, err := GetItem("/", auth)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Fatal("Could not fetch root item of filesystem!")
	}
	root.cache = cache
	cache.root = root.ID()
	cache.InsertID(cache.root, root)

	// using token=latest because we don't care about existing items - they'll
	// be downloaded on-demand by the cache
	cache.deltaLink = "/me/drive/root/delta?token=latest"

	// deltaloop is started manually
	return cache
}

// GetAuth returns the current auth
func (c *Cache) GetAuth() *Auth {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.auth
}

func leadingSlash(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// InodePath calculates an inode's path to the filesystem root
func (c *Cache) InodePath(inode *fs.Inode) string {
	root, _ := c.GetPath("/", nil)
	return leadingSlash(inode.Path(root.EmbeddedInode()))
}

// GetID gets an item from the cache by ID. No fetching is performed. Result is
// nil if no item is found.
func (c *Cache) GetID(id string) *DriveItem {
	entry, exists := c.metadata.Load(id)
	if !exists {
		return nil
	}
	item := entry.(*DriveItem)
	return item
}

// InsertID inserts a single item into the cache by ID and sets its parent using
// the Item.Parent.ID, if set. Must be called after DeleteID, if being used to
// rename/move an item.
func (c *Cache) InsertID(id string, item *DriveItem) {
	// make sure the item knows about the cache itself, then insert
	item.mutex.Lock()
	item.cache = c
	item.mutex.Unlock()
	c.metadata.Store(id, item)

	parentID := item.ParentID()
	if parentID == "" {
		// root item, or parent not set
		return
	}
	parent := c.GetID(parentID)
	if parent == nil {
		log.WithFields(log.Fields{
			"parentID":  parentID,
			"childID":   id,
			"childName": item.Name(),
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
	if item.IsDir() {
		parent.subdir++
	}
	parent.children = append(parent.children, item.ID())
}

// InsertChild adds an item as a child of a specified parent ID.
func (c *Cache) InsertChild(parentID string, child *DriveItem) {
	child.mutex.Lock()
	// should already be set, just double-checking here.
	child.APIItem.Parent.ID = parentID
	id := child.IDInternal
	child.mutex.Unlock()
	c.InsertID(id, child)
}

// DeleteID deletes an item from the cache, and removes it from its parent. Must
// be called before InsertID if being used to rename/move an item.
func (c *Cache) DeleteID(id string) {
	if item := c.GetID(id); item != nil {
		parent := c.GetID(item.ParentID())
		parent.mutex.Lock()
		for i, childID := range parent.children {
			if childID == id {
				parent.children = append(parent.children[:i], parent.children[i+1:]...)
				if item.IsDir() {
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
	Children []*DriveItem `json:"value"`
}

// GetChild fetches a named child of an item. Wraps GetChildrenID.
func (c *Cache) GetChild(id string, name string, auth *Auth) (*DriveItem, error) {
	children, err := c.GetChildrenID(id, auth)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.Name() == name {
			return child, nil
		}
	}
	return nil, errors.New("Child does not exist")
}

// GetChildrenID grabs all DriveItems that are the children of the given ID. If
// items are not found, they are fetched.
func (c *Cache) GetChildrenID(id string, auth *Auth) (map[string]*DriveItem, error) {
	// fetch item and catch common errors
	item := c.GetID(id)
	children := make(map[string]*DriveItem)
	if item == nil {
		log.WithFields(log.Fields{
			"id": id,
		}).Error("Item not found in cache")
		return children, errors.New(id + " not found in cache")
	} else if !item.IsDir() {
		// Normal files are treated as empty folders. This only gets called if
		// we messed up and tried to get the children of a plain-old file.
		log.WithFields(log.Fields{
			"id":   id,
			"path": item.Path(),
		}).Warn("Attepted to get children of ordinary file")
		return children, nil
	}

	// If item.children is not nil, it means we have the item's children
	// already and can fetch them directly from the cache
	if item.children != nil {
		for _, id := range item.children {
			child := c.GetID(id)
			if child == nil {
				// will be nil if deleted or never existed
				continue
			}
			children[strings.ToLower(child.Name())] = child
		}
		return children, nil
	}

	// check that we have a valid auth before proceeding
	if auth == nil || auth.AccessToken == "" {
		return nil, errors.New("Auth was nil/zero and children of \"" +
			item.Path() +
			"\" were not in cache. Could not fetch item as a result.")
	}

	// We haven't fetched the children for this item yet, get them from the
	// server.
	body, err := Get(ChildrenPathID(id), auth)
	var fetched driveChildren
	if err != nil {
		return nil, err
	}
	json.Unmarshal(body, &fetched)

	item.mutex.Lock()
	item.children = make([]string, 0)
	for _, child := range fetched.Children {
		// we will always have an id after fetching from the server
		c.metadata.Store(child.IDInternal, child)

		// store in result map
		children[strings.ToLower(child.Name())] = child

		// store id in parent item and increment parents subdirectory count
		item.children = append(item.children, child.IDInternal)
		if child.IsDir() {
			item.subdir++
		}
	}
	item.mutex.Unlock()

	return children, nil
}

// GetChildrenPath grabs all DriveItems that are the children of the resource at
// the path. If items are not found, they are fetched.
func (c *Cache) GetChildrenPath(path string, auth *Auth) (map[string]*DriveItem, error) {
	item, err := c.GetPath(path, auth)
	if err != nil {
		return make(map[string]*DriveItem), err
	}

	return c.GetChildrenID(item.ID(), auth)
}

// GetPath fetches a given DriveItem in the cache, if any items along the way are
// not found, they are fetched.
func (c *Cache) GetPath(path string, auth *Auth) (*DriveItem, error) {
	lastID := c.root
	if path == "/" {
		return c.GetID(lastID), nil
	}

	// from the root directory, traverse the chain of items till we reach our
	// target ID.
	path = strings.TrimSuffix(strings.ToLower(path), "/")
	split := strings.Split(path, "/")[1:] //omit leading "/"
	var item *DriveItem
	for i := 0; i < len(split); i++ {
		// fetches children
		children, err := c.GetChildrenID(lastID, auth)
		if err != nil {
			return nil, err
		}

		var exists bool // if we use ":=", item is shadowed
		item, exists = children[split[i]]
		if !exists {
			// the item still doesn't exist after fetching from server. it
			// doesn't exist
			return nil, errors.New(strings.Join(split[:i+1], "/") +
				" does not exist on server or in local cache")
		}
		lastID = item.ID()
	}
	return item, nil
}

// DeletePath an item from the cache by path. Must be called before Insert if
// being used to move/rename an item.
func (c *Cache) DeletePath(key string) {
	item, _ := c.GetPath(strings.ToLower(key), nil)
	if item != nil {
		c.DeleteID(item.ID())
	}
}

// InsertPath lets us manually insert an item to the cache (like if it was
// created locally). Overwrites a cached item if present. Must be called after
// delete if being used to move/rename an item.
func (c *Cache) InsertPath(key string, auth *Auth, item *DriveItem) error {
	key = strings.ToLower(key)

	// set the item.Parent.ID properly if the item hasn't been in the cache
	// before or is being moved.
	parent, err := c.GetPath(filepath.Dir(key), auth)
	if err != nil {
		return err
	} else if parent == nil {
		log.WithFields(log.Fields{
			"key":  key,
			"path": item.Path(),
		}).Error("Parent of key was nil! Did we accidentally use an ID for the key?")
		return errors.New("Parent of key was nil! Did we accidentally use an ID for the key?")
	}

	// Coded this way to make sure locks are in the same order for the deadlock
	// detector (lock ordering needs to be the same as InsertID: Parent->Child).
	parentID := parent.ID()
	item.mutex.Lock()
	item.APIItem.Parent.ID = parentID
	item.mutex.Unlock()

	c.InsertID(item.ID(), item)
	return nil
}

// MoveID moves an item to a new ID name. Also responsible for handling the
// actual overwrite of the item's IDInternal field
func (c *Cache) MoveID(oldID string, newID string) error {
	item := c.GetID(oldID)
	if item == nil {
		// It may have already been renamed. This is not an error. We assume
		// that IDs will never collide. Re-perform the op if this is the case.
		if item = c.GetID(newID); item == nil {
			// nope, it just doesn't exist
			return errors.New("Could not get item: " + oldID)
		}
	}

	// need to rename the child under the parent
	parent := c.GetID(item.ParentID())
	parent.mutex.Lock()
	for i, child := range parent.children {
		if child == oldID {
			parent.children[i] = newID
			break
		}
	}
	parent.mutex.Unlock()

	item.mutex.Lock()
	item.IDInternal = newID
	item.mutex.Unlock()

	c.DeleteID(oldID)
	c.InsertID(newID, item)
	return nil
}

// MovePath an item to a new position
func (c *Cache) MovePath(oldPath string, newPath string, auth *Auth) error {
	item, err := c.GetPath(oldPath, auth)
	if err != nil {
		return err
	}

	c.DeletePath(oldPath)
	if newBase := filepath.Base(newPath); filepath.Base(oldPath) != newBase {
		item.SetName(newBase)
	}
	if err := c.InsertPath(newPath, auth, item); err != nil {
		// insert failed, reinsert in old location
		item.SetName(filepath.Base(oldPath))
		c.InsertPath(oldPath, auth, item)
		return err
	}
	return nil
}

// GetContent read a file's content from disk.
func (c *Cache) GetContent(id string) []byte {
	var content []byte // nil
	c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("content"))
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
		b := tx.Bucket([]byte("content"))
		return b.Put([]byte(id), content)
	})
}

// DeleteContent deletes content from disk.
func (c *Cache) DeleteContent(id string) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("content"))
		return b.Delete([]byte(id))
	})
}
