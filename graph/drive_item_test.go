package graph

import (
	"io/ioutil"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
)

// verify that items automatically get created with an ID of "local-"
func TestConstructor(t *testing.T) {
	item := NewDriveItem("Test Create", 0644|fuse.S_IFREG, nil)
	if item.ID() == "" || !isLocalID(item.ID()) {
		t.Fatalf("Expected an ID beginning with \"local-\", got \"%s\" instaed",
			item.ID())
	}
}

// verify that the mode of items fetched are correctly set when fetched from
// server
func TestMode(t *testing.T) {
	item, _ := GetItemPath("/Documents", auth)
	if item.Mode() != uint32(0755|fuse.S_IFDIR) {
		t.Fatalf("mode of /Documents wrong: %o != %o",
			item.Mode(), 0755|fuse.S_IFDIR)
	}

	fname := "/onedriver_tests/test_mode.txt"
	failOnErr(t, ioutil.WriteFile("mount"+fname, []byte("test"), 0644))
	item, _ = GetItemPath(fname, auth)
	if item.Mode() != uint32(0644|fuse.S_IFREG) {
		t.Fatalf("mode of file wrong: %o != %o",
			item.Mode(), 0644|fuse.S_IFREG)
	}
}

// Do we properly detect whether something is a directory or not?
func TestIsDir(t *testing.T) {
	item, _ := GetItemPath("/Documents", auth)
	if !item.IsDir() {
		t.Fatal("/Documents not detected as a directory")
	}

	fname := "/onedriver_tests/test_is_dir.txt"
	failOnErr(t, ioutil.WriteFile("mount"+fname, []byte("test"), 0644))
	item, _ = GetItemPath(fname, auth)
	if item.IsDir() {
		t.Fatal("file created with mode 644 not detected as a file")
	}
}
