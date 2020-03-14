package fs

import (
	"time"

	"github.com/jstaf/onedriver/fs/graph"
)

// NewFS is basically a wrapper around NewCache, but with a dedicated thread to
// poll the server for changes.
func NewFS(dbPath string, authPath string, deltaInterval time.Duration) *Inode {
	auth := graph.Authenticate(authPath)
	cache := NewCache(auth, dbPath)
	root, _ := cache.GetPath("/", auth)
	go cache.deltaLoop(deltaInterval)
	return root
}
