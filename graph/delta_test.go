package graph

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Run tests to verify that we are syncing changes from the server.
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
