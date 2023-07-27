package fs

import (
	"crypto/md5"
	"fmt"
	"os"
)

const (
	ThumbnailSizeLarge  = "large"
	ThumbnailSizeNormal = "normal"
	ThumbnailSizeFail   = "fail/gnome-thumbnail-factory"
)

type Thumbnailer struct {
	mountpoint string
}

// HashURI hashes a URI like the Freedesktop standard expects
// https://specifications.freedesktop.org/thumbnail-spec/thumbnail-spec-latest.html
func HashURI(uri string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(uri)))
}

// ThumbnailPath generates the path for local thumbnail
func (t Thumbnailer) ThumbnailPath(path string, thumbnailSize string) string {
	cacheDir, _ := os.UserCacheDir()
	uri := fmt.Sprintf("file://%s%s", t.mountpoint, path)
	return fmt.Sprintf("%s/thumbnails/%s/%s.jpg", cacheDir, thumbnailSize, HashURI(uri))
}
