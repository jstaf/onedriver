// these tests are independent of the mounted fs
package fs

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootGet(t *testing.T) {
	t.Parallel()
	cache := NewFilesystem(auth, filepath.Join(testDBLoc, "test_root_get"))
	root, err := cache.GetPath("/", auth)
	require.NoError(t, err)
	assert.Equal(t, "/", root.Path(), "Root path did not resolve correctly.")
}

func TestRootChildrenUpdate(t *testing.T) {
	t.Parallel()
	cache := NewFilesystem(auth, filepath.Join(testDBLoc, "test_root_children_update"))
	children, err := cache.GetChildrenPath("/", auth)
	require.NoError(t, err)

	if _, exists := children["documents"]; !exists {
		t.Fatal("Could not find documents folder.")
	}
}

func TestSubdirGet(t *testing.T) {
	t.Parallel()
	cache := NewFilesystem(auth, filepath.Join(testDBLoc, "test_subdir_get"))
	documents, err := cache.GetPath("/Documents", auth)
	require.NoError(t, err)
	assert.Equal(t, "Documents", documents.Name(), "Failed to fetch \"/Documents\".")
}

func TestSubdirChildrenUpdate(t *testing.T) {
	t.Parallel()
	cache := NewFilesystem(auth, filepath.Join(testDBLoc, "test_subdir_children_update"))
	children, err := cache.GetChildrenPath("/Documents", auth)
	require.NoError(t, err)

	if _, exists := children["documents"]; exists {
		fmt.Println("Documents directory found inside itself. " +
			"Likely the cache did not traverse correctly.\n\nChildren:")
		for key := range children {
			fmt.Println(key)
		}
		t.FailNow()
	}
}

func TestSamePointer(t *testing.T) {
	t.Parallel()
	cache := NewFilesystem(auth, filepath.Join(testDBLoc, "test_same_pointer"))
	item, _ := cache.GetPath("/Documents", auth)
	item2, _ := cache.GetPath("/Documents", auth)
	if item != item2 {
		t.Fatalf("Pointers to cached items do not match: %p != %p\n", item, item2)
	}
	assert.NotNil(t, item)
}
