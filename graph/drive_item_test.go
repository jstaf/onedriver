package graph

import (
	"testing"

	"github.com/hanwen/go-fuse/fuse"
)

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

// Do we properly detect whether something is a directory or not
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

func TestGetChildren(t *testing.T) {
	root, _ := GetItem("/", auth)
	items, err := root.GetChildren(auth)
	if err != nil {
		t.Fatal(err)
	}

	var success bool
	for _, item := range items {
		if item.Name() == "Documents" {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("Could not find the '/Documents' folder as a child of '/'!")
	}
}
