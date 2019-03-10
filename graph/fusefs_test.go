package graph

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// test that both mkdir and rmdir work, as well as the potentially failing
// mkdir->rmdir->mkdir chaing caused by a bad cache
func TestMkdirRmdir(t *testing.T) {
	fname := "folder1"
	failOnErr(t, exec.Command("mkdir", filepath.Join(TestDir, fname)).Run())
	failOnErr(t, exec.Command("rmdir", filepath.Join(TestDir, fname)).Run())
	failOnErr(t, exec.Command("mkdir", filepath.Join(TestDir, fname)).Run())
}
