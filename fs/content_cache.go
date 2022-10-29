package fs

import (
	"os"
	"path/filepath"
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

// DeleteContent deletes content from disk.
func (l *LoopbackCache) Delete(id string) error {
	return os.Remove(l.contentPath(id))
}

// MoveContent moves content from one ID to another
func (l *LoopbackCache) Move(oldID string, newID string) error {
	return os.Rename(l.contentPath(oldID), l.contentPath(newID))
}

// OpenContent returns a filehandle for subsequent access
func (l *LoopbackCache) Open(id string) (*os.File, error) {
	if fd, ok := l.fds.Load(id); ok {
		// already opened, return existing fd
		return fd.(*os.File), nil
	}

	fd, err := os.OpenFile(l.contentPath(id), os.O_RDWR, 0700)
	if err != nil {
		return nil, err
	}
	l.fds.Store(id, fd)
	return fd, err
}

func (l *LoopbackCache) Close(id string) {
	if fd, ok := l.fds.Load(id); ok {
		fd.(*os.File).Close()
		l.fds.Delete(id)
	}
}

// Write performs a normal file write. Returns number of bytes read and error.
func (l *LoopbackCache) Write(id string, offset int64, data []byte) (int, error) {
	fd, err := l.Open(id)
	if err != nil {
		return 0, err
	}
	return fd.WriteAt(data, offset)
}

// Read performs a normal file read. Returns number of bytes read and any error encountered.
func (l LoopbackCache) Read(id string, offset int64, output []byte) (int, error) {
	fd, err := l.Open(id)
	if err != nil {
		return 0, err
	}
	return fd.ReadAt(output, offset)
}
