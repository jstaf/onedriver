package fs

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// newOfflineUploadManager creates an UploadManager backed by a temporary
// database and an otherwise empty Filesystem. It uses an empty auth, so every
// upload it starts fails right away without making any network requests.
func newOfflineUploadManager(t *testing.T, tick time.Duration) (*UploadManager, *Filesystem) {
	t.Helper()
	db, err := bolt.Open(filepath.Join(t.TempDir(), "offline.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketMetadata)
		return err
	}))
	f := &Filesystem{db: db}
	return NewUploadManager(tick, db, f, &graph.Auth{}), f
}

// isUploadPersisted checks whether an upload session is stored in the db.
func isUploadPersisted(u *UploadManager, id string) bool {
	found := false
	u.db.View(func(tx *bolt.Tx) error {
		if b := tx.Bucket(bucketUploads); b != nil {
			found = b.Get([]byte(id)) != nil
		}
		return nil
	})
	return found
}

// An upload that keeps failing used to leak one in-flight slot per retry, so
// after it was given up on no other upload was ever started. Also checks that
// a file whose upload was given up on is kept marked as modified.
func TestUploadManagerRetriesDoNotStallUploads(t *testing.T) {
	t.Parallel()
	u, f := newOfflineUploadManager(t, 10*time.Millisecond)

	// uploads are run one after another, so the second one can only start if
	// the retries of the first one did not use up all slots
	for i := 0; i < 2; i++ {
		inode := NewInode(fmt.Sprintf("retry_%d.txt", i), 0644, nil)
		f.metadata.Store(inode.ID(), inode)
		data := []byte("this upload is going to fail")
		session, err := NewUploadSession(inode, &data)
		require.NoError(t, err)
		u.queue <- session

		require.Eventually(t, inode.HasChanges, 10*time.Second, 10*time.Millisecond,
			"Upload %d was never given up on and marked as modified again.", i)
		assert.False(t, isUploadPersisted(u, inode.ID()),
			"Upload %d was given up on but is still persisted.", i)
	}
}
