package fs

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/jstaf/onedriver/fs/graph"

	bolt "go.etcd.io/bbolt"
)

// Test that new uploads are written to disk to support resuming them later if
// the user shuts down their computer.
func TestUploadDiskSerialization(t *testing.T) {
	t.Parallel()
	// write a file and get its id
	failOnErr(t, ioutil.WriteFile(filepath.Join(TestDir, "upload_to_disk.txt"), []byte("cheesecake"), 0644))
	inode, err := fsCache.GetPath("/onedriver_tests/upload_to_disk.txt", nil)
	failOnErr(t, err)

	// we can find the in-progress upload because there is a several second
	// delay on new uploads
	session := UploadSession{}
	failOnErr(t, fsCache.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(UPLOADS)
		if b == nil {
			return errors.New("uploads bucket did not exist")
		}
		diskSession := b.Get([]byte(inode.ID()))
		if diskSession == nil {
			return errors.New("Item to upload not found on disk")
		}
		return json.Unmarshal(diskSession, &session)
	}))

	// kill the session before it gets uploaded
	fsCache.uploads.CancelUpload(session.ID)

	// confirm that the file didn't get uploaded yet (just in case!)
	driveItem, err := graph.GetItemPath("/onedriver_tests/upload_to_disk.txt", auth)
	if err == nil || driveItem != nil {
		if driveItem.Size > 0 {
			t.Fatal("This test should be rewritten, the file was uploaded before " +
				"the upload could be canceled.")
		}
	}

	// now we create a new UploadManager from scratch, with the file injected
	// into its db and confirm that the file gets uploaded
	db, err := bolt.Open("test_upload_disk_serialization.db", 0644, nil)
	failOnErr(t, err)
	db.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucket(UPLOADS)
		payload, _ := json.Marshal(&session)
		return b.Put([]byte(session.ID), payload)
	})

	NewUploadManager(time.Second, db, auth)
	time.Sleep(10 * time.Second)
	driveItem, err = graph.GetItemPath("/onedriver_tests/upload_to_disk.txt", auth)
	if err != nil || driveItem == nil {
		t.Fatal("Could not find uploaded file after unserializing from disk and resuming upload.")
	}
	if driveItem.Size == 0 {
		t.Fatal("Size was 0 - the upload was never completed.")
	}
}

// There are apparently some edge cases where an upload can remain 0 bytes, even
// after a successful upload. We need to monitor for these cases and mark these
// as failed so they can be retried. TODO: this test sucks and should be rewritten
func TestUploadZeroSizeRetry(t *testing.T) {
	t.Parallel()

	// write a file and get its id
	contents := []byte("fudge")
	failOnErr(t, ioutil.WriteFile(filepath.Join(TestDir, "upload_fail_empty.txt"), contents, 0644))
	inode, err := fsCache.GetPath("/onedriver_tests/upload_fail_empty.txt", nil)
	failOnErr(t, err)

	// kill the session before it gets uploaded
	fsCache.uploads.CancelUpload(inode.ID())

	// verify its size is still 0 server-side
	remote, err := graph.GetItem(inode.ID(), auth)
	failOnErr(t, err)
	if remote == nil {
		t.Fatal("A placeholder file was never uploaded for upload_fail_empty.txt." +
			"This is a bug in this test.")
	}
	if remote.Size > 0 {
		t.Fatal("The file finished its upload before it could be canceled. " +
			"This is a bug in this test.")
	}

	// create a new session, check the remote checksums the way the app would, then
	// upload manually and re-check
	inode.data = &contents
	session, err := NewUploadSession(inode, auth)
	if session.verifyRemoteChecksum(auth) {
		t.Fatal("Checksums should not match before reupload.")
	}

	failOnErr(t, session.Upload(auth))
	if !session.verifyRemoteChecksum(auth) {
		t.Fatal("Checksums must match post upload.")
	}
}
