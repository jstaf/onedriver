package fs

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// newOfflineFilesystem creates a Filesystem with only a root folder, backed by
// a temporary cache directory. Its UploadManager ticks at the given interval
// and has an empty auth, so any upload it starts fails without making network
// requests.
func newOfflineFilesystem(t *testing.T, tick time.Duration) (*Filesystem, *Inode) {
	t.Helper()
	dir := t.TempDir()
	db, err := bolt.Open(filepath.Join(dir, "offline.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketMetadata)
		return err
	}))

	f := &Filesystem{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
		content:       NewLoopbackCache(filepath.Join(dir, "content")),
		db:            db,
		auth:          &graph.Auth{},
		opendirs:      make(map[uint64][]*Inode),
	}
	root := NewInodeDriveItem(&graph.DriveItem{
		ID:     "offline-root",
		Name:   "root",
		Parent: &graph.DriveItemParent{},
		Folder: &graph.Folder{},
	})
	f.root = root.ID()
	f.InsertID(f.root, root)
	f.uploads = NewUploadManager(tick, db, f, f.auth)
	return f, root
}

// persistedUpload returns the upload session stored in the db under an ID.
func persistedUpload(t *testing.T, f *Filesystem, id string) *UploadSession {
	t.Helper()
	var session *UploadSession
	require.NoError(t, f.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketUploads)
		if b == nil {
			return nil
		}
		if data := b.Get([]byte(id)); data != nil {
			session = &UploadSession{}
			return json.Unmarshal(data, session)
		}
		return nil
	}))
	return session
}

// A delta can report a new file's server ID before the upload manager has
// noticed that the upload which created it completed. MoveID used to cancel the
// item's pending upload in that case, losing a rewrite of the file that was
// queued while the first upload was running.
func TestMoveIDKeepsPendingUpload(t *testing.T) {
	t.Parallel()
	f, root := newOfflineFilesystem(t, time.Hour)

	inode := NewInode("rewritten.txt", 0644|fuse.S_IFREG, root)
	localID := inode.ID()
	f.InsertID(localID, inode)
	data := []byte("content written during the first upload")
	require.NoError(t, f.content.Insert(localID, data))
	session, err := NewUploadSession(inode, &data)
	require.NoError(t, err)
	f.uploads.queue <- session

	const serverID = "offline-server-id-rewrite"
	require.NoError(t, f.MoveID(localID, serverID))

	assert.Equal(t, inode, f.GetID(serverID))
	assert.Equal(t, serverID, inode.ID())
	assert.Nil(t, persistedUpload(t, f, localID), "Upload still stored under the old ID.")
	moved := persistedUpload(t, f, serverID)
	if assert.NotNil(t, moved, "Pending upload was not moved to the new ID.") {
		assert.Equal(t, serverID, moved.ID)
		assert.Equal(t, serverID, moved.OldID)
		assert.Equal(t, data, moved.Data)
	}
}

// If a file is renamed or moved on the server before we learned its server ID,
// a delta cannot match it by name and adds a second inode for it. MoveID used
// to overwrite that inode with ours, losing the new name and parent, and left
// the item listed twice in its parent.
func TestMoveIDMergesInodeAddedByDelta(t *testing.T) {
	t.Parallel()
	f, root := newOfflineFilesystem(t, time.Hour)

	dir := NewInodeDriveItem(&graph.DriveItem{
		ID:     "offline-dir",
		Name:   "dir",
		Parent: &graph.DriveItemParent{ID: root.ID()},
		Folder: &graph.Folder{},
	})
	f.InsertChild(root.ID(), dir)

	inode := NewInode("before.txt", 0644|fuse.S_IFREG, root)
	localID := inode.ID()
	f.InsertID(localID, inode)
	require.NoError(t, f.content.Insert(localID, []byte("uploaded content")))

	// what applyDelta adds for the renamed and moved item
	const serverID = "offline-server-id-rename"
	f.InsertChild(dir.ID(), NewInodeDriveItem(&graph.DriveItem{
		ID:     serverID,
		Name:   "after.txt",
		Parent: &graph.DriveItemParent{ID: dir.ID()},
		File:   &graph.File{},
	}))

	require.NoError(t, f.MoveID(localID, serverID))

	assert.Equal(t, inode, f.GetID(serverID))
	assert.Equal(t, "after.txt", inode.Name())
	assert.Equal(t, dir.ID(), inode.ParentID())
	assert.Equal(t, []byte("uploaded content"), f.content.Get(serverID))

	root.RLock()
	assert.NotContains(t, root.children, localID)
	assert.NotContains(t, root.children, serverID)
	root.RUnlock()
	dir.RLock()
	assert.Equal(t, []string{serverID}, dir.children)
	dir.RUnlock()
}

// When the upload loop notices that an upload produced a new ID, it moves the
// item itself. It cannot use MoveID for this, which waits for the upload loop.
func TestUploadCompletionMovesID(t *testing.T) {
	t.Parallel()
	f, root := newOfflineFilesystem(t, 10*time.Millisecond)

	inode := NewInode("completed.txt", 0644|fuse.S_IFREG, root)
	localID := inode.ID()
	f.InsertID(localID, inode)
	data := []byte("uploaded content")
	require.NoError(t, f.content.Insert(localID, data))
	session, err := NewUploadSession(inode, &data)
	require.NoError(t, err)

	// pretend the upload is running, so the upload loop does not start it
	session.setState(uploadStarted, nil)
	f.uploads.queue <- session
	require.Eventually(t, func() bool {
		return persistedUpload(t, f, localID) != nil
	}, 10*time.Second, 10*time.Millisecond)

	// what a successful Upload leaves behind
	const serverID = "offline-server-id-completed"
	session.Lock()
	session.ID = serverID
	session.ETag = "offline-etag"
	session.Unlock()
	session.setState(uploadComplete, nil)

	require.Eventually(t, func() bool {
		inode.RLock()
		etag := inode.DriveItem.ETag
		inode.RUnlock()
		// the session is removed after the ETag is set
		return f.GetID(serverID) == inode && etag == "offline-etag" &&
			persistedUpload(t, f, localID) == nil &&
			persistedUpload(t, f, serverID) == nil
	}, 10*time.Second, 10*time.Millisecond, "Upload completion was not handled.")
	assert.Equal(t, serverID, inode.ID())
	assert.Equal(t, data, f.content.Get(serverID))
}
