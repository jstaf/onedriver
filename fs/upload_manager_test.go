package fs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// Test that new uploads are written to disk to support resuming them later if
// the user shuts down their computer.
func TestUploadDiskSerialization(t *testing.T) {
	t.Parallel()
	// write a file and get its id - we do this as a goroutine because uploads are
	// blocking now
	go exec.Command("cp", "dmel.fa", filepath.Join(TestDir, "upload_to_disk.fa")).Run()
	time.Sleep(time.Second)
	inode, err := fs.GetPath("/onedriver_tests/upload_to_disk.fa", nil)
	require.NoError(t, err)

	// we can find the in-progress upload because there is a several second
	// delay on new uploads
	session := UploadSession{}
	err = fs.db.Batch(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucketIfNotExists(bucketUploads)
		diskSession := b.Get([]byte(inode.ID()))
		if diskSession == nil {
			return errors.New("item to upload not found on disk")
		}
		return json.Unmarshal(diskSession, &session)
	})
	if err != nil {
		t.Log(err)
		t.Log("This test sucks and should be rewritten to be less race-y!")
		t.SkipNow()
	}

	// kill the session before it gets uploaded
	fs.uploads.CancelUpload(session.ID)

	// confirm that the file didn't get uploaded yet (just in case!)
	driveItem, err := graph.GetItemPath("/onedriver_tests/upload_to_disk.fa", auth)
	if err == nil || driveItem != nil {
		if driveItem.Size > 0 {
			t.Fatal("This test should be rewritten, the file was uploaded before " +
				"the upload could be canceled.")
		}
	}

	// now we create a new UploadManager from scratch, with the file injected
	// into its db and confirm that the file gets uploaded
	db, err := bolt.Open(filepath.Join(testDBLoc, "test_upload_disk_serialization.db"), 0644, nil)
	require.NoError(t, err)
	db.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucket(bucketUploads)
		payload, _ := json.Marshal(&session)
		return b.Put([]byte(session.ID), payload)
	})

	NewUploadManager(time.Second, db, fs, auth)
	assert.Eventually(t, func() bool {
		driveItem, err = graph.GetItemPath("/onedriver_tests/upload_to_disk.fa", auth)
		if driveItem != nil && err == nil {
			return true
		}
		return false
	}, 100*time.Second, 5*time.Second,
		"Could not find uploaded file after unserializing from disk and resuming upload.",
	)
}

// Make sure that uploading the same file multiple times works exactly as it should.
func TestRepeatedUploads(t *testing.T) {
	t.Parallel()

	// test setup
	fname := filepath.Join(TestDir, "repeated_upload.txt")
	require.NoError(t, os.WriteFile(fname, []byte("initial content"), 0644))
	var inode *Inode
	require.Eventually(t, func() bool {
		inode, _ = fs.GetPath("/onedriver_tests/repeated_upload.txt", auth)
		return !isLocalID(inode.ID())
	}, retrySeconds, 2*time.Second, "ID was local after upload.")

	for i := 0; i < 5; i++ {
		uploadme := []byte(fmt.Sprintf("iteration: %d", i))
		require.NoError(t, os.WriteFile(fname, uploadme, 0644))

		time.Sleep(5 * time.Second)

		item, err := graph.GetItemPath("/onedriver_tests/repeated_upload.txt", auth)
		require.NoError(t, err)

		content, _, err := graph.GetItemContent(item.ID, auth)
		require.NoError(t, err)

		if !bytes.Equal(content, uploadme) {
			// wait and retry once
			time.Sleep(5 * time.Second)
			content, _, err := graph.GetItemContent(item.ID, auth)
			require.NoError(t, err)
			if !bytes.Equal(content, uploadme) {
				t.Fatalf("Upload failed - got \"%s\", wanted \"%s\"", content, uploadme)
			}
		}
	}
}
