package fs

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUploadSession verifies that the basic functionality of uploads works correctly.
func TestUploadSession(t *testing.T) {
	t.Parallel()
	testDir, err := fs.GetPath("/onedriver_tests", auth)
	require.NoError(t, err)

	inode := NewInode("uploadSessionSmall.txt", 0644, testDir)
	data := []byte("our super special data")
	inode.setContent(data)
	mtime := inode.ModTime()

	session, err := NewUploadSession(inode, &data)
	require.NoError(t, err)
	err = session.Upload(auth)
	require.NoError(t, err)
	if isLocalID(session.ID) {
		t.Fatalf("The session's ID was somehow still local following an upload: %s\n",
			session.ID)
	}
	if sessionMtime := uint64(session.ModTime.Unix()); sessionMtime != mtime {
		t.Errorf("session modtime changed - before: %d - after: %d", mtime, sessionMtime)
	}

	resp, _, err := graph.GetItemContent(session.ID, auth)
	require.NoError(t, err)
	if !bytes.Equal(data, resp) {
		t.Fatalf("Data mismatch. Original content: %s\nRemote content: %s\n", data, resp)
	}

	// item now has a new id following the upload. We just change the ID here
	// because thats part of the UploadManager functionality and gets tested elsewhere.
	inode.DriveItem.ID = session.ID

	// we overwrite and upload again to test uploading with the new remote id
	newData := []byte("new data is extra long so it covers the old one completely")
	inode.setContent(newData)

	session2, err := NewUploadSession(inode, &newData)
	require.NoError(t, err)
	err = session2.Upload(auth)
	require.NoError(t, err)

	resp, _, err = graph.GetItemContent(session.ID, auth)
	require.NoError(t, err)
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
	require.NoError(t, err)

	time.Sleep(10 * time.Second)
	item, err := graph.GetItemPath("/onedriver_tests/uploadSessionSmallFS.txt", auth)
	if err != nil || item == nil {
		t.Fatal(err)
	}

	content, _, err := graph.GetItemContent(item.ID, auth)
	require.NoError(t, err)
	if !bytes.Equal(content, data) {
		t.Fatalf("Data mismatch. Original content: %s\nRemote content: %s\n", data, content)
	}

	// upload it again to ensure uploads with an existing remote id succeed
	data = []byte("more super special data")
	err = ioutil.WriteFile(filepath.Join(TestDir, "uploadSessionSmallFS.txt"), data, 0644)
	require.NoError(t, err)

	time.Sleep(15 * time.Second)
	item2, err := graph.GetItemPath("/onedriver_tests/uploadSessionSmallFS.txt", auth)
	if err != nil || item == nil {
		t.Fatal(err)
	}

	content, _, err = graph.GetItemContent(item2.ID, auth)
	require.NoError(t, err)
	if !bytes.Equal(content, data) {
		t.Fatalf("Data mismatch. Original content: %s\nRemote content: %s\n", data, content)
	}
}

// copy large file inside onedrive mount, then verify that we can still
// access selected lines
func TestUploadSessionLargeFS(t *testing.T) {
	t.Parallel()
	fname := filepath.Join(TestDir, "dmel.fa")
	require.NoError(t, exec.Command("cp", "dmel.fa", fname).Run())

	contents, err := ioutil.ReadFile(fname)
	require.NoError(t, err)

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
	var item *graph.DriveItem
	assert.Eventually(t, func() bool {
		item, _ = graph.GetItemPath("/onedriver_tests/dmel.fa", auth)
		inode := NewInodeDriveItem(item)
		return item != nil && inode.Size() == size
	}, 120*time.Second, time.Second, "Upload session did not complete successfully!")

	// test multipart downloads as a bonus part of the test
	downloaded, _, err := graph.GetItemContent(item.ID, auth)
	assert.NoError(t, err)
	assert.Equal(t, graph.SHA1Hash(&contents), graph.SHA1Hash(&downloaded),
		"Downloaded content did not match original content.")
}
