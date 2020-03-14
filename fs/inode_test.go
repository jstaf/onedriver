package fs

import (
	"io/ioutil"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
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
	failOnErr(t, ioutil.WriteFile("mount"+fname, []byte("test"), 0644))
	item, _ = graph.GetItemPath(fname, auth)
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
	failOnErr(t, ioutil.WriteFile("mount"+fname, []byte("test"), 0644))
	item, _ = graph.GetItemPath(fname, auth)
	inode = NewInodeDriveItem(item)
	if inode.IsDir() {
		t.Fatal("file created with mode 644 not detected as a file")
	}
}
