package fs

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashURI(t *testing.T) {
	require.Equal(t,
		HashURI("file:///home/jstaf/OneDrive/ginger.jpg"),
		"fa0e529cb193135a04bdd3ec4ced1f44",
	)
}

func TestThumbnailPath(t *testing.T) {
	onedriveFilePath := "/ginger.jpg"
	thumbnailer := Thumbnailer{mountpoint: "/tmp/test"}
	mountedFilePath := fmt.Sprintf("file://%s%s", thumbnailer.mountpoint, onedriveFilePath)

	cacheDir, _ := os.UserCacheDir()
	require.Equal(t,
		fmt.Sprintf(
			"%s/thumbnails/%s/%s.jpg",
			cacheDir,
			ThumbnailSizeLarge,
			HashURI(mountedFilePath),
		),
		thumbnailer.ThumbnailPath(onedriveFilePath, ThumbnailSizeLarge),
	)
}
