package fs

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// LoopbackCache stores the content for files under a folder as regular files
type LoopbackCache struct {
	directory string
	fds       sync.Map
}

func NewLoopbackCache(directory string) *LoopbackCache {
	os.Mkdir(directory, 0700)
	return &LoopbackCache{
		directory: directory,
		fds:       sync.Map{},
	}
}

// contentPath returns the path for the given content file
func (l *LoopbackCache) contentPath(id string) string {
	return filepath.Join(l.directory, id)
}

// GetContent reads a file's content from disk.
func (l *LoopbackCache) Get(id string) []byte {
	content, _ := os.ReadFile(l.contentPath(id))
	return content
}

// InsertContent writes file content to disk in a single bulk insert.
func (l *LoopbackCache) Insert(id string, content []byte) error {
	return os.WriteFile(l.contentPath(id), content, 0600)
}

// InsertStream inserts a stream of data
func (l *LoopbackCache) InsertStream(id string, reader io.Reader) (int64, error) {
	fd, err := l.Open(id)
	if err != nil {
		return 0, err
	}
	return io.Copy(fd, reader)
}

// DeleteContent deletes content from disk.
func (l *LoopbackCache) Delete(id string) error {
	return os.Remove(l.contentPath(id))
}

// MoveContent moves content from one ID to another
func (l *LoopbackCache) Move(oldID string, newID string) error {
	return os.Rename(l.contentPath(oldID), l.contentPath(newID))
}

// HasContent is used to find if we have a file or not in cache (in any state)
func (l *LoopbackCache) HasContent(id string) bool {
	// is it already open?
	_, ok := l.fds.Load(id)
	if ok {
		return ok
	}
	// is it on disk?
	_, err := os.Stat(l.contentPath(id))
	return err == nil
}

// OpenContent returns a filehandle for subsequent access
func (l *LoopbackCache) Open(id string) (*os.File, error) {
	if fd, ok := l.fds.Load(id); ok {
		// already opened, return existing fd
		return fd.(*os.File), nil
	}

	fd, err := os.OpenFile(l.contentPath(id), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	// Since we explicitly want to store *os.Files, we need to prevent the Go
	// GC from trying to be "helpful" and closing files for us behind the
	// scenes.
	// https://github.com/hanwen/go-fuse/issues/371#issuecomment-694799535
	runtime.SetFinalizer(fd, nil)
	l.fds.Store(id, fd)
	return fd, nil
}

func (l *LoopbackCache) Close(id string) {
	if fd, ok := l.fds.Load(id); ok {
		fd.(*os.File).Close()
		l.fds.Delete(id)
	}
}

// Size returns the size of the object in the cache
func (l *LoopbackCache) Size(id string) int64 {
	st, err := os.Stat(l.contentPath(id))
	if err != nil {
		return -1
	}
	return st.Size()
}
