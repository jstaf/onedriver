package fs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestThumbnailPath(t *testing.T) {
	require.Equal(t,
		"file:///home/jstaf/OneDrive/ginger.jpg",
		"fa0e529cb193135a04bdd3ec4ced1f44.png",
	)
}
