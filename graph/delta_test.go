// Run tests to verify that we are syncing changes from the server.
package graph

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// In this test, we create a directory through the API, and wait to see if
// the cache picks it up post-creation.
func TestDeltaMkdir(t *testing.T) {
	t.Parallel()
	parent, err := GetItemPath("/onedriver_tests/delta", auth)
	failOnErr(t, err)

	// create the directory directly through the API and bypass the cache
	_, err = Mkdir("first", parent.ID(), auth)
	failOnErr(t, err)

	// give the delta thread time to fetch the item
	time.Sleep(10 * time.Second)
	st, err := os.Stat(filepath.Join(DeltaDir, "first"))
	failOnErr(t, err)
	if !st.Mode().IsDir() {
		t.Fatalf("%s was not a directory!", filepath.Join(DeltaDir, "first"))
	}
}

// We create a directory through the cache, then delete through the API and see
// if the cache picks it up.
func TestDeltaRmdir(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(DeltaDir, "delete_me")
	failOnErr(t, os.Mkdir(fname, 0755))

	item, err := GetItemPath("/onedriver_tests/delta/delete_me", auth)
	failOnErr(t, err)
	failOnErr(t, Remove(item.ID(), auth))

	// wait for delta sync
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second)
		_, err := os.Stat(fname)
		if err != nil {
			// this is what we actually want
			return
		}
	}
	t.Fatal("File deletion not picked up by client")
}

// Create a file locally, then rename it remotely and verify that the renamed
// file still has the correct content under the new parent.
func TestDeltaRename(t *testing.T) {
	t.Parallel()
	failOnErr(t, ioutil.WriteFile(
		filepath.Join(DeltaDir, "delta_rename_start"),
		[]byte("cheesecake"),
		0644,
	))

	item, err := GetItemPath("/onedriver_tests/delta/delta_rename_start", auth)
	failOnErr(t, err)

	failOnErr(t, Rename(item.ID(), "delta_rename_end", item.ParentID(), auth))
	for i := 0; i < 10; i++ {
		fpath := filepath.Join(DeltaDir, "delta_rename_end")
		if _, err := os.Stat(fpath); err != nil {
			content, err := ioutil.ReadFile(fpath)
			failOnErr(t, err)
			if bytes.Contains(content, []byte("cheesecake")) {
				return
			}
		}
	}
	t.Fatal("Rename not detected by client.")
}

// Create a file locally, then move it on the server to a new directory. Check
// to see if the cache picks it up.
func TestDeltaMoveParent(t *testing.T) {
	t.Parallel()
	failOnErr(t, ioutil.WriteFile(
		filepath.Join(DeltaDir, "delta_move_start"),
		[]byte("carrotcake"),
		0644,
	))

	item, err := GetItemPath("/onedriver_tests/delta/delta_move_start", auth)
	failOnErr(t, err)

	newParent, err := GetItemPath("/onedriver_tests/", auth)
	failOnErr(t, err)

	failOnErr(t, Rename(item.ID(), "delta_rename_end", newParent.ID(), auth))
	for i := 0; i < 10; i++ {
		fpath := filepath.Join(TestDir, "delta_rename_end")
		if _, err := os.Stat(fpath); err != nil {
			content, err := ioutil.ReadFile(fpath)
			failOnErr(t, err)
			if bytes.Contains(content, []byte("carrotcake")) {
				return
			}
		}
	}
	t.Fatal("Rename not detected by client.")
}
