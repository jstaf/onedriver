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

	manager := NewUploadManager(time.Second, db, auth)
	if _, exists := manager.sessions[session.ID]; !exists {
		t.Fatal("Could not find session after unmarshaling from disk.")
	}

	time.Sleep(5 * time.Second)
	driveItem, err = graph.GetItemPath("/onedriver_tests/upload_to_disk.txt", auth)
	if err != nil || driveItem == nil {
		t.Fatal("Could not find uploaded file after unserializing from disk and resuming upload.")
	}
}
