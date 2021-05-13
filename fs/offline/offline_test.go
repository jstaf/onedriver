package offline

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/jstaf/onedriver/fs"
)

// We should see more than zero items when we run ls.
func TestOfflineReaddir(t *testing.T) {
	t.Parallel()
	files, err := ioutil.ReadDir(TestDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) == 0 {
		t.Fatal("Expected more than 0 files in the test mount directory.")
	}
}

// We should find the file named bagels (from TestEchoWritesToFile)
func TestOfflineBagelDetection(t *testing.T) {
	t.Parallel()
	files, err := ioutil.ReadDir(TestDir)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, f := range files {
		if f.Name() == "bagels" {
			found = true
			if f.IsDir() {
				t.Fatal("\"bagels\" should be an ordinary file, not a directory")
			}
			octal := fs.Octal(uint32(f.Mode().Perm()))
			if octal[0] != '6' || int(octal[1])-4 < 0 || octal[2] != '4' {
				// middle bit just needs to be higher than 4
				// for compatibility with 022 / 002 umasks on different distros
				t.Fatalf("\"bagels\" permissions bits wrong, got %s, expected 644", octal)
			}
			break
		}
	}
	if !found {
		t.Fatal("\"bagels\" not found! Expected file not present.")
	}
}

// Does the contents of the bagels file match what it should?
func TestOfflineBagelContents(t *testing.T) {
	t.Parallel()
	contents, err := ioutil.ReadFile(filepath.Join(TestDir, "bagels"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, []byte("bagels\n")) {
		t.Fatalf("Did not find \"bagels\", got %s instead", string(contents))
	}
}

// Creating a file should fail
func TestOfflineFileCreation(t *testing.T) {
	t.Parallel()
	if ioutil.WriteFile(filepath.Join(TestDir, "donuts"), []byte("fail me"), 0644) == nil {
		t.Fatal("Writing a file while offline should fail.")
	}
}

// Modifying a file offline should fail.
func TestOfflineFileModification(t *testing.T) {
	t.Parallel()
	if ioutil.WriteFile(filepath.Join(TestDir, "bagels"), []byte("fail me too"), 0644) == nil {
		t.Fatal("Modifying a file while offline should fail.")
	}
}

// Deleting a file offline should fail.
func TestOfflineFileDeletion(t *testing.T) {
	t.Parallel()
	if os.Remove(filepath.Join(TestDir, "write.txt")) == nil {
		t.Fatal("Deleting a file while offline should fail.")
	}
	if os.Remove(filepath.Join(TestDir, "empty")) == nil {
		t.Fatal("Deleting an empty file while offline should fail.")
	}
}

// Creating a directory offline should fail.
func TestOfflineMkdir(t *testing.T) {
	t.Parallel()
	if os.Mkdir(filepath.Join(TestDir, "offline_dir"), 0755) == nil {
		t.Fatal("Creating a directory should have failed offline.")
	}
}

// Deleting a directory offline should fail.
func TestOfflineRmdir(t *testing.T) {
	t.Parallel()
	if os.Remove(filepath.Join(TestDir, "folder1")) == nil {
		t.Fatal("Removing a directory should have failed offline.")
	}
}
