package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetItem(t *testing.T) {
	t.Parallel()
	var auth Auth
	auth.FromFile(".auth_tokens.json")
	item, err := GetItemPath("/", &auth)
	assert.NoError(t, err)
	assert.Equal(t, "root", item.Name, "Failed to fetch directory root.")

	_, err = GetItemPath("/lkjfsdlfjdwjkfl", &auth)
	assert.Error(t, err, "We didn't return an error for a non-existent item!")
}

func TestChunkRanges(t *testing.T) {
	t.Parallel()
	const c = downloadChunkSize
	assert.Empty(t, chunkRanges(0))
	assert.Equal(t, [][2]uint64{{0, c - 1}}, chunkRanges(c))
	assert.Equal(t, [][2]uint64{{0, c - 1}, {c, c}}, chunkRanges(c+1))
	// exact multiples must not request a range past the end of the file
	assert.Equal(t, [][2]uint64{{0, c - 1}, {c, 2*c - 1}}, chunkRanges(2*c))
	assert.Equal(t, [][2]uint64{{0, c - 1}, {c, 2*c - 1}, {2 * c, 3*c - 1}}, chunkRanges(3*c))
}
