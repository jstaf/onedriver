package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetItem(t *testing.T) {
	t.Parallel()
	var auth Auth
	auth.FromFile(".auth_tokens.json")
	item, err := GetItemPath(Me, "/", &auth)
	require.NoError(t, err)
	assert.Equal(t, "root", item.Name, "Failed to fetch directory root.")

	_, err = GetItemPath(Me, "/lkjfsdlfjdwjkfl", &auth)
	assert.Error(t, err, "We didn't return an error for a non-existent item!")
}
