package graph

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// does ls work and can we find the Documents/Pictures folders
func TestLS(t *testing.T) {
	stdout, err := exec.Command("ls", "mount").Output()
	failOnErr(t, err)
	sout := string(stdout)
	if !strings.Contains(sout, "Documents") {
		t.Fatal("Could not find \"Documents\" folder.")
	}
	if !strings.Contains(sout, "Documents") {
		t.Fatal("Could not find \"Pictures\" folder.")
	}
}

// can touch create an empty file
func TestTouchCreate(t *testing.T) {
	fname := filepath.Join(TestDir, "empty")
	failOnErr(t, exec.Command("touch", fname).Run())
	st, err := os.Stat(fname)
	failOnErr(t, err)
	if st.Size() != 0 {
		t.Fatal("size was not 0")
	}
	if st.Mode() != 0644 {
		t.Fatal("Mode of new file was not 644")
	}
	if st.IsDir() {
		t.Fatal("New file detected as directory")
	}
}

// does the touch command update modification time properly?
func TestTouchUpdateTime(t *testing.T) {
	fname := filepath.Join(TestDir, "modtime")
	failOnErr(t, exec.Command("touch", fname).Run())
	st1, _ := os.Stat(fname)

	time.Sleep(2 * time.Second)

	failOnErr(t, exec.Command("touch", fname).Run())
	st2, _ := os.Stat(fname)

	if st2.ModTime().Equal(st1.ModTime()) || st2.ModTime().Before(st1.ModTime()) {
		t.Fatalf("File modification time was not updated by touch:\n"+
			"Before: %d\nAfter: %d\n", st1.ModTime().Unix(), st2.ModTime().Unix())
	}
}

// chmod should *just work*
func TestChmod(t *testing.T) {
	fname := filepath.Join(TestDir, "chmod_tester")
	failOnErr(t, exec.Command("touch", fname).Run())
	failOnErr(t, os.Chmod(fname, 0777))
	st, _ := os.Stat(fname)
	if st.Mode() != 0777 {
		t.Fatalf("Mode of file was not 0777, got %o instead", st.Mode())
	}
}

// test that both mkdir and rmdir work, as well as the potentially failing
// mkdir->rmdir->mkdir chain that fails if the cache hangs on to an old copy
// after rmdir
func TestMkdirRmdir(t *testing.T) {
	fname := filepath.Join(TestDir, "folder1")
	failOnErr(t, exec.Command("mkdir", fname).Run())
	failOnErr(t, exec.Command("rmdir", fname).Run())
	failOnErr(t, exec.Command("mkdir", fname).Run())
}
