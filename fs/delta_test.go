// Run tests to verify that we are syncing changes from the server.
package fs

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
)

const retrySeconds = 15

// In this test, we create a directory through the API, and wait to see if
// the cache picks it up post-creation.
func TestDeltaMkdir(t *testing.T) {
	t.Parallel()
	parent, err := graph.GetItemPath("/onedriver_tests/delta", auth)
	failOnErr(t, err)

	// create the directory directly through the API and bypass the cache
	_, err = graph.Mkdir("first", parent.ID, auth)
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

	item, err := graph.GetItemPath("/onedriver_tests/delta/delete_me", auth)
	failOnErr(t, err)
	failOnErr(t, graph.Remove(item.ID, auth))

	// wait for delta sync
	for i := 0; i < retrySeconds; i++ {
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

	item, err := graph.GetItemPath("/onedriver_tests/delta/delta_rename_start", auth)
	failOnErr(t, err)
	inode := NewInodeDriveItem(item)

	failOnErr(t, graph.Rename(inode.ID(), "delta_rename_end", inode.ParentID(), auth))
	fpath := filepath.Join(DeltaDir, "delta_rename_end")
	for i := 0; i < retrySeconds; i++ {
		time.Sleep(time.Second)
		if _, err := os.Stat(fpath); err == nil {
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
	time.Sleep(time.Second)

	item, err := graph.GetItemPath("/onedriver_tests/delta/delta_move_start", auth)
	failOnErr(t, err)

	newParent, err := graph.GetItemPath("/onedriver_tests/", auth)
	failOnErr(t, err)

	failOnErr(t, graph.Rename(item.ID, "delta_rename_end", newParent.ID, auth))
	fpath := filepath.Join(TestDir, "delta_rename_end")
	for i := 0; i < retrySeconds; i++ {
		time.Sleep(time.Second)
		if _, err := os.Stat(fpath); err == nil {
			content, err := ioutil.ReadFile(fpath)
			failOnErr(t, err)
			if bytes.Contains(content, []byte("carrotcake")) {
				return
			}
		}
	}
	t.Fatal("Rename not detected by client.")
}

// Change the content remotely on the server, and verify it gets propagated to
// to the client.
func TestDeltaContentChangeRemote(t *testing.T) {
	t.Parallel()
	failOnErr(t, ioutil.WriteFile(
		filepath.Join(DeltaDir, "remote_content"),
		[]byte("the cake is a lie"),
		0644,
	))

	// change and upload it via the API
	time.Sleep(time.Second * 10)
	item, err := graph.GetItemPath("/onedriver_tests/delta/remote_content", auth)
	inode := NewInodeDriveItem(item)
	failOnErr(t, err)
	newContent := []byte("because it has been changed remotely!")
	inode.DriveItem.Size = uint64(len(newContent))
	inode.data = &newContent
	session, err := NewUploadSession(inode, auth)
	failOnErr(t, err)
	failOnErr(t, session.Upload(auth))

	time.Sleep(time.Second * 5)
	body, _ := graph.GetItemContent(inode.ID(), auth)
	if bytes.Compare(body, newContent) != 0 {
		t.Fatalf("Failed to upload test file. Remote content: \"%s\"", body)
	}

	var content []byte
	for i := 0; i < retrySeconds; i++ {
		time.Sleep(time.Second)
		content, err = ioutil.ReadFile(filepath.Join(DeltaDir, "remote_content"))
		failOnErr(t, err)
		if bytes.Compare(content, newContent) == 0 {
			return
		}
	}

	t.Fatalf("Failed to sync content to local machine. Got content: \"%s\". "+
		"Wanted: \"because it has been changed remotely!\". "+
		"Remote content: \"%s\".",
		string(content), string(body))
}

// Change the content both on the server and the client and verify that the
// client data is preserved.
func TestDeltaContentChangeBoth(t *testing.T) {
	t.Parallel()
	fpath := filepath.Join(DeltaDir, "both_content_changed")
	failOnErr(t, ioutil.WriteFile(fpath, []byte("initial content"), 0644))

	// change and upload it via the API
	item, err := graph.GetItemPath("/onedriver_tests/delta/both_content_changed", auth)
	inode := NewInodeDriveItem(item)
	failOnErr(t, err)
	newContent := []byte("remote")
	inode.data = &newContent
	session, err := NewUploadSession(inode, auth)
	failOnErr(t, err)
	failOnErr(t, session.Upload(auth))

	// now change it locally
	failOnErr(t, ioutil.WriteFile(fpath, []byte("local"), 0644))

	// file has been changed both remotely and locally
	time.Sleep(time.Second * retrySeconds)
	content, err := ioutil.ReadFile(fpath)
	failOnErr(t, err)

	if bytes.Equal(content, []byte("local")) {
		return
	}
	t.Fatal("Client copy not preserved")
}

// If we have local content in the local disk cache that doesn't match what the
// server has, Open() should pick this up and wipe it. Otherwise Open() could
// pick up an old version of a file from previous program startups and think
// it's current, which would erase the real, up-to-date server copy.
func TestDeltaBadContentInCache(t *testing.T) {
	t.Parallel()
	// write a file to the server and poll until it exists
	failOnErr(t, ioutil.WriteFile(
		filepath.Join(DeltaDir, "corrupted"),
		[]byte("correct contents"),
		0644,
	))
	var id string
	for i := 0; i < retrySeconds; i++ {
		time.Sleep(time.Second)
		item, err := graph.GetItemPath("/onedriver_tests/delta/corrupted", auth)
		if err == nil {
			id = item.ID
			break
		}
	}

	fsCache.InsertContent(id, []byte("wrong contents"))
	contents, err := ioutil.ReadFile(filepath.Join(DeltaDir, "corrupted"))
	failOnErr(t, err)
	if bytes.HasPrefix(contents, []byte("wrong")) {
		t.Fatalf("File contents were wrong! Got \"%s\", wanted \"correct contents\"",
			string(contents))
	}
}
