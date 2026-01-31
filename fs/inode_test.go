package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// verify that items automatically get created with an ID of "local-"
func TestConstructor(t *testing.T) {
	t.Parallel()
	inode := NewInode("Test Create", 0644|fuse.S_IFREG, nil)
	if inode.ID() == "" || !isLocalID(inode.ID()) {
		t.Fatalf("Expected an ID beginning with \"local-\", got \"%s\" instaed",
			inode.ID())
	}
}

// verify that the mode of items fetched are correctly set when fetched from
// server
func TestMode(t *testing.T) {
	t.Parallel()
	item, _ := graph.GetItemPath("/Documents", auth)
	inode := NewInodeDriveItem(item)
	if inode.Mode() != uint32(0755|fuse.S_IFDIR) {
		t.Fatalf("mode of /Documents wrong: %o != %o",
			inode.Mode(), 0755|fuse.S_IFDIR)
	}

	fname := "/onedriver_tests/test_mode.txt"
	require.NoError(t, os.WriteFile("mount"+fname, []byte("test"), 0644))

	var err error
	for i := 0; i < 10; i++ {
		item, err = graph.GetItemPath(fname, auth)
		if err == nil && item != nil {
			break
		}
		time.Sleep(time.Second)
	}
	if item == nil {
		t.Fatal("item cannot be nil, err:", err)
	}
	inode = NewInodeDriveItem(item)
	if inode.Mode() != uint32(0644|fuse.S_IFREG) {
		t.Fatalf("mode of file wrong: %o != %o",
			inode.Mode(), 0644|fuse.S_IFREG)
	}
}

// Do we properly detect whether something is a directory or not?
func TestIsDir(t *testing.T) {
	t.Parallel()
	item, _ := graph.GetItemPath("/Documents", auth)
	inode := NewInodeDriveItem(item)
	if !inode.IsDir() {
		t.Fatal("/Documents not detected as a directory")
	}

	fname := "/onedriver_tests/test_is_dir.txt"
	require.NoError(t, os.WriteFile("mount"+fname, []byte("test"), 0644))

	assert.Eventually(t, func() bool {
		item, err := graph.GetItemPath(fname, auth)
		if err == nil && item != nil {
			if inode := NewInodeDriveItem(item); inode.IsDir() {
				t.Fatal("File created with mode 644 not detected as file")
			}
			return true
		}
		return false
	}, 10*time.Second, time.Second, "Could not create item.")
}

// A filename like .~lock.libreoffice-test.docx# will fail to upload unless the
// filename is escaped.
func TestFilenameEscape(t *testing.T) {
	t.Parallel()
	fname := `.~lock.libreoffice-test.docx#`
	require.NoError(t, os.WriteFile(filepath.Join(TestDir, fname), []byte("argl bargl"), 0644))

	// make sure it made it to the server
	assert.Eventually(t, func() bool {
		children, err := graph.GetItemChildrenPath("/onedriver_tests", auth)
		require.NoError(t, err)
		for _, child := range children {
			if child.Name == fname {
				return true
			}
		}
		return false
	}, retrySeconds, 5*time.Second, "Could not find file: ", fname)
}

// When running creat() on an existing file, we should truncate the existing file and
// return the original inode.
// Related to: https://github.com/jstaf/onedriver/issues/99
func TestDoubleCreate(t *testing.T) {
	t.Parallel()
	fname := "double_create.txt"

	parent, err := fs.GetPath("/onedriver_tests", auth)
	require.NoError(t, err)

	fs.Create(
		context.Background().Done(),
		&fuse.CreateIn{
			InHeader: fuse.InHeader{NodeId: parent.NodeID()},
			Mode:     0644,
		},
		fname,
		&fuse.CreateOut{},
	)
	child, err := fs.GetChild(parent.ID(), fname, auth)

	// we clean up after ourselves to prevent failing some of the offline tests
	defer fs.Unlink(context.Background().Done(), &fuse.InHeader{NodeId: parent.nodeID}, fname)

	if err != nil || child == nil {
		t.Fatal("Could not find child post-create")
	}
	childID := child.ID()

	fs.Create(
		context.Background().Done(),
		&fuse.CreateIn{
			InHeader: fuse.InHeader{NodeId: parent.NodeID()},
			Mode:     0644,
		},
		fname,
		&fuse.CreateOut{},
	)
	child, err = fs.GetChild(parent.ID(), fname, auth)
	require.NoError(t, err)
	if child == nil {
		t.Fatal("Could not find child post-create")
	}
	assert.Equal(t, childID, child.ID(),
		"IDs did not match when create run twice on same file.",
	)
}
