package graph

import "time"

// NewFS is basically a wrapper around NewCache, but with a dedicated thread to
// poll the server for changes.
func NewFS(dbpath string, deltaInterval time.Duration) *Inode {
	auth := Authenticate()
	cache := NewCache(auth, dbpath)
	root, _ := cache.GetPath("/", auth)
	go cache.deltaLoop(deltaInterval)
	return root
}
