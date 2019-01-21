package graph

import "testing"

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
