package graph

import "testing"

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

/*
// appending to a file via "echo test >> file" should add to the end and not
// truncate it at like 5 bytes
func TestWriteAppend(t *testing.T) {}
*/

/*
// Writes within the file should not truncate it, and overwite data properly
func TestWriteOverwrite(t *testing.T) {}
*/
