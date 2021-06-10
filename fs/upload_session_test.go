package fs

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
)

// TestUploadSessionSmall verifies that the basic functionality of uploads works correctly.
func TestUploadSessionSmall(t *testing.T) {
	t.Parallel()
	testDir, err := fsCache.GetPath("/onedriver_tests", auth)
	failOnErr(t, err)

	inode := NewInode("uploadSessionSmall.txt", 0644, testDir)
	data := []byte("our super special data")
	if _, errno := inode.Write(context.Background(), nil, data, 0); errno != 0 {
		t.Fatalf("Could not write to inode, errno: %d\n", errno)
	}

	session, err := NewUploadSession(inode)
	failOnErr(t, err)
	err = session.Upload(auth)
	failOnErr(t, err)
	if isLocalID(session.ID) {
		t.Fatalf("The session's ID was somehow still local following an upload: %s\n",
			session.ID)
	}

	resp, err := graph.GetItemContent(session.ID, auth)
	failOnErr(t, err)
	if !bytes.Equal(data, resp) {
		t.Fatalf("Data mismatch. Original content: %s\nRemote content: %s\n", data, resp)
	}

	// we overwrite and upload again to test uploading with the new remote id
	newData := []byte("new data is extra long so it covers the old one completely")
	if _, errno := inode.Write(context.Background(), nil, newData, 0); errno != 0 {
		t.Fatalf("Could not write to inode, errno: %d\n", errno)
	}

	session2, err := NewUploadSession(inode)
	failOnErr(t, err)
	err = session2.Upload(auth)
	failOnErr(t, err)

	resp, err = graph.GetItemContent(session.ID, auth)
	failOnErr(t, err)
	if !bytes.Equal(newData, resp) {
		t.Fatalf("Data mismatch. Original content: %s\nRemote content: %s\n", newData, resp)
	}
}

// TestUploadSessionSmallFS verifies is the same test as TestUploadSessionSmall, but uses
// the filesystem itself to perform the uploads instead of testing the internal upload
// functions directly
func TestUploadSessionSmallFS(t *testing.T) {
	t.Parallel()
	data := []byte("super special data for upload test 2")
	err := ioutil.WriteFile(filepath.Join(TestDir, "uploadSessionSmallFS.txt"), data, 0644)
	failOnErr(t, err)

	time.Sleep(5 * time.Second)
	item, err := graph.GetItemPath("/onedriver_tests/uploadSessionSmallFS.txt", auth)
	if err != nil || item == nil {
		t.Fatal(err)
	}

	content, err := graph.GetItemContent(item.ID, auth)
	failOnErr(t, err)
	if !bytes.Equal(content, data) {
		t.Fatalf("Data mismatch. Original content: %s\nRemote content: %s\n", data, content)
	}

	// upload it again to ensure uploads with an existing remote id succeed
	data = []byte("more super special data")
	err = ioutil.WriteFile(filepath.Join(TestDir, "uploadSessionSmallFS.txt"), data, 0644)
	failOnErr(t, err)

	time.Sleep(15 * time.Second)
	item2, err := graph.GetItemPath("/onedriver_tests/uploadSessionSmallFS.txt", auth)
	if err != nil || item == nil {
		t.Fatal(err)
	}

	content, err = graph.GetItemContent(item2.ID, auth)
	failOnErr(t, err)
	if !bytes.Equal(content, data) {
		t.Fatalf("Data mismatch. Original content: %s\nRemote content: %s\n", data, content)
	}
}

// copy large file inside onedrive mount, then verify that we can still
// access selected lines
func TestUploadSessionLargeFS(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "dmel.fa")
	failOnErr(t, exec.Command("cp", "dmel.fa", fname).Run())

	contents, err := ioutil.ReadFile(fname)
	failOnErr(t, err)

	header := ">X dna:chromosome chromosome:BDGP6.22:X:1:23542271:1 REF"
	if string(contents[:len(header)]) != header {
		t.Fatalf("Could not read FASTA header. Wanted \"%s\", got \"%s\"\n",
			header, string(contents[:len(header)]))
	}

	final := "AAATAAAATAC\n" // makes yucky test output, but is the final line
	match := string(contents[len(contents)-len(final):])
	if match != final {
		t.Fatalf("Could not read final line of FASTA. Wanted \"%s\", got \"%s\"\n",
			final, match)
	}

	st, _ := os.Stat(fname)
	if st.Size() == 0 {
		t.Fatal("File size cannot be 0.")
	}

	// poll endpoint to make sure it has a size greater than 0
	size := uint64(len(contents))
	for i := 0; i < 120; i++ {
		time.Sleep(time.Second)
		item, _ := graph.GetItemPath("/onedriver_tests/dmel.fa", auth)
		inode := NewInodeDriveItem(item)
		if item != nil && inode.Size() == size {
			return
		}
	}
	t.Fatalf("\nUpload session did not complete successfully!")
}
