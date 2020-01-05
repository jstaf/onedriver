package offline

import (
	"io/ioutil"
	"testing"
)

// We should see more than zero items when we run ls.
func TestOfflineReaddir(t *testing.T) {
	files, err := ioutil.ReadDir(TestDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) == 0 {
		t.Fatal("Expected more than 0 files in the test mount directory.")
	}
}
