package fs

import (
	"crypto/md5"
	"fmt"
)

// ThumbnailPath generates the path for local thumbnail
func (f *Filesystem) ThumbnailPath(path string) string {
	//TODO fixme
	uri := fmt.Sprintf("file://%s/%s", f.content.directory, path)
	return fmt.Sprintf("%x", md5.Sum([]byte(uri)))
}
