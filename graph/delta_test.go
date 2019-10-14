// Run tests to verify that we are syncing changes from the server.
package graph

import (
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
