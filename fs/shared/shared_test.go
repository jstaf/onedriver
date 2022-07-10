package shared

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jstaf/onedriver/fs"
	"github.com/stretchr/testify/assert"
)

func checkForIOError(t *testing.T, err error) {
	if err != nil && strings.Contains(err.Error(), "remote I/O error") {
		t.Errorf("We should refuse ops with something besides an I/O error.\nGot:\n%s\n", err)
	}
}

// We should not be able to modify or delete an immutable inode.
func TestImmutableInodes(t *testing.T) {
	t.Parallel()
	err := os.Rename(fs.TestSharedDir, "mount/Shared")
	assert.Error(t, err, "Renaming the immutable shared folder should fail.")
	checkForIOError(t, err)

	err = os.Remove(fs.TestSharedDir)
	assert.Error(t, err, "Deleting the immutable shared folder should fail.")
	checkForIOError(t, err)
}

// We should not be able to create new children in immutable inodes (at least the shared
// folder one).
func TestImmutableInodesNoChildren(t *testing.T) {
	t.Parallel()
	err := os.Mkdir(filepath.Join(fs.TestSharedDir, "failureFolder"), 0755)
	assert.Error(t, err, "Creating a folder in the immutable shared directory should fail!")
	checkForIOError(t, err)

	err = ioutil.WriteFile(
		filepath.Join(fs.TestSharedDir, "failureFile.txt"),
		[]byte("This should fail.\n"),
		0644,
	)
	assert.Error(t, err, "Creating a file in the immutable shared directory should fail!")
	checkForIOError(t, err)
}
