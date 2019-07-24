package graph

import (
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
	item, _ := GetItem("/Documents", auth)
	if item.Mode() != uint32(0755|fuse.S_IFDIR) {
		t.Fatalf("mode of /Documents wrong: %o != %o",
			item.Mode(), 0755|fuse.S_IFDIR)
	}

	item, _ = GetItem("/Getting Started with Onedrive.pdf", auth)
	if item.Mode() != uint32(0644|fuse.S_IFREG) {
		t.Fatalf("mode of intro PDF wrong: %o != %o",
			item.Mode(), 0644|fuse.S_IFREG)
	}
}

// Do we properly detect whether something is a directory or not?
func TestIsDir(t *testing.T) {
	item, _ := GetItem("/Documents", auth)
	if !item.IsDir() {
		t.Fatal("/Documents not detected as a directory")
	}
	item, _ = GetItem("/Getting Started with Onedrive.pdf", auth)
	if item.IsDir() {
		t.Fatal("Intro to Onedrive.pdf not detected as a file")
	}
}
