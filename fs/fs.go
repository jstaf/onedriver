package fs

import (
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
	log "github.com/sirupsen/logrus"
)

const timeout = time.Second

// Filesystem is the actual FUSE filesystem and uses the go analogy of the
// "low-level" FUSE API here:
// https://github.com/libfuse/libfuse/blob/master/include/fuse_lowlevel.h
type Filesystem struct {
	fuse.RawFileSystem
	cache *Cache
	auth  *graph.Auth

	m          sync.RWMutex
	lastNodeID uint64
	inodes     []string

	// tracks currently open directories
	opendirsM sync.RWMutex
	opendirs  map[uint64][]*Inode
}

// NewFilesystem creates a new filesystem
func NewFilesystem(cacheDir string, auth *graph.Auth) *Filesystem {
	fs := &Filesystem{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
		cache:         NewCache(auth, filepath.Join(cacheDir, "onedriver.db")),
		auth:          auth,
		opendirs:      make(map[uint64][]*Inode),
	}
	// root inode is inode 1
	fs.insertInode(fs.cache.GetID(fs.cache.root))
	return fs
}

// insertInodeID assigns a session-specific inode ID to the item if not already
// set. Does nothing if called on a pre-existing item. These IDs are reset
// every time the filesystem restarts.
func (f *Filesystem) insertInode(inode *Inode) uint64 {
	if nid := inode.NodeID(); nid > 0 {
		return nid
	}

	f.m.Lock()
	defer f.m.Unlock()
	f.lastNodeID++
	inode.SetNodeID(f.lastNodeID)
	f.inodes = append(f.inodes, inode.ID())
	return f.lastNodeID
}

// getInodeID fetches the DriveItemID for a given inode id
func (f *Filesystem) getInodeID(nodeID uint64) string {
	f.m.RLock()
	defer f.m.RUnlock()
	if nodeID > f.lastNodeID {
		return ""
	}
	return f.inodes[nodeID-1]
}

// ReadDir provides a list of all the entries in the directory
func (f *Filesystem) OpenDir(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	id := f.getInodeID(input.NodeId)
	dir := f.cache.GetID(id)
	if dir == nil {
		return fuse.ENOENT
	}
	if !dir.IsDir() {
		return fuse.ENOTDIR
	}
	log.WithFields(log.Fields{
		"nodeID": input.NodeId,
		"id":     id,
		"name":   dir.Name(),
	}).Debug()

	children, err := f.cache.GetChildrenID(id, f.auth)
	if err != nil {
		// not an item not found error (Lookup/Getattr will always be called
		// before Readdir()), something has happened to our connection
		log.WithFields(log.Fields{
			"id":  id,
			"err": err,
		}).Error("Error during ReadDir")
		return fuse.EREMOTEIO
	}

	parent := f.cache.GetID(dir.ParentID())
	if parent == nil {
		// This is the parent of the mountpoint. The FUSE kernel module discards
		// this info, so what we put here doesn't actually matter.
		parent = NewInode("..", 0755|fuse.S_IFDIR, nil)
		parent.nodeID = math.MaxUint64
	}

	entries := make([]*Inode, 2)
	entries[0] = dir
	entries[1] = parent

	for _, child := range children {
		f.insertInode(child)
		entries = append(entries, child)
	}
	f.opendirsM.Lock()
	f.opendirs[input.NodeId] = entries
	f.opendirsM.Unlock()

	return fuse.OK
}

// ReleaseDir closes a directory and purges it from memory
func (f *Filesystem) ReleaseDir(input *fuse.ReleaseIn) {
	f.opendirsM.Lock()
	delete(f.opendirs, input.NodeId)
	f.opendirsM.Unlock()
}

// ReadDirPlus reads an individual directory entry AND does a lookup.
func (f *Filesystem) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	f.opendirsM.RLock()
	entries, ok := f.opendirs[input.NodeId]
	f.opendirsM.RUnlock()
	if !ok {
		return fuse.EBADF
	}

	if input.Offset >= uint64(len(entries)) {
		// just tried to seek past end of directory, we're all done!
		return fuse.OK
	}

	inode := entries[input.Offset]
	entry := fuse.DirEntry{
		Ino:  inode.NodeID(),
		Mode: inode.Mode(),
	}
	// first two entries will always be "." and ".."
	switch input.Offset {
	case 0:
		entry.Name = "."
	case 1:
		entry.Name = ".."
	default:
		entry.Name = inode.Name()
	}

	entryOut := out.AddDirLookupEntry(entry)
	entryOut.Attr = inode.makeattr()
	entryOut.SetAttrTimeout(timeout)
	entryOut.SetEntryTimeout(timeout)
	return fuse.OK
}

// Lookup is called by the kernel when the VFS wants to know about a file inside
// a directory.
func (f *Filesystem) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	id := f.getInodeID(header.NodeId)
	log.WithFields(log.Fields{
		"nodeID": header.NodeId,
		"id":     id,
		"name":   name,
	}).Trace()

	child, _ := f.cache.GetChild(id, strings.ToLower(name), f.auth)
	if child == nil {
		return fuse.ENOENT
	}

	out.NodeId = child.NodeID()
	out.Attr = child.makeattr()
	out.SetAttrTimeout(timeout)
	out.SetEntryTimeout(timeout)
	return fuse.OK
}

func (f *Filesystem) GetAttr(cancel <-chan struct{}, input *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	id := f.getInodeID(input.NodeId)
	log.WithFields(log.Fields{
		"nodeID": input.NodeId,
		"id":     id,
	}).Trace()

	inode := f.cache.GetID(id)
	if inode == nil {
		return fuse.ENOENT
	}

	out.Attr = inode.makeattr()
	out.SetTimeout(timeout)
	return fuse.OK
}
