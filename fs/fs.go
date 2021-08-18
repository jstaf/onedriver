package fs

import (
	"github.com/hanwen/go-fuse/v2/fuse"
)

type Filesystem struct {
	fuse.RawFileSystem
}

func NewFilesystem() *Filesystem {
	return &Filesystem{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
	}
}
