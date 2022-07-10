package fs

import (
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/stretchr/testify/assert"
)

// throw a bunch of inodes in and verify that we pull things out in the right order
func TestInodeMapping(t *testing.T) {
	t.Parallel()
	mapper := InodeMapper{}
	inode := NewInode("inode1", 0644|fuse.S_IFREG, nil)
	inode2 := NewInode("inode2", 0644|fuse.S_IFREG, nil)
	nodeID1 := mapper.AssignNodeID(inode)
	nodeID2 := mapper.AssignNodeID(inode2)
	assert.EqualValues(t, 1, nodeID1, "Expected inode number 1")
	assert.Equal(t, inode.ID(), mapper.MapNodeID(1), "IDs did not match")
	assert.EqualValues(t, 2, nodeID2, "Expected inode number 2")
	assert.Equal(t, inode2.ID(), mapper.MapNodeID(2), "IDs did not match")
}

// inserting the same inode repeatedly should result in the same nodeids
func TestRepeatedInodeInsert(t *testing.T) {
	t.Parallel()
	mapper := InodeMapper{}
	inode := NewInode("inode", 0644|fuse.S_IFREG, nil)
	originalID := mapper.AssignNodeID(inode)
	for i := 0; i < 10; i++ {
		newID := mapper.AssignNodeID(inode)
		assert.Equal(t, originalID, newID, "NodeIDs should not change")
	}
}

// verify that ids are properly reassigned if they change
func TestLocalIDReassignment(t *testing.T) {
	t.Parallel()
	mapper := InodeMapper{}
	inode := NewInode("inode", 0644|fuse.S_IFREG, nil)

	originalNodeID := mapper.AssignNodeID(inode)

	newID := "ajdslfkajdfj"
	inode.DriveItem.ID = newID

	newNodeID := mapper.AssignNodeID(inode)
	assert.Equal(t, originalNodeID, newNodeID,
		"Node IDs should not change on ID reassignment.")
	assert.Equal(t, newID, mapper.MapNodeID(newNodeID),
		"Expected new the newly set ID, not the original local one.")
}

// verify that we handle out of bound nodeids properly
func TestNodeIDOutOfBounds(t *testing.T) {
	t.Parallel()
	mapper := InodeMapper{}
	mapper.AssignNodeID(NewInode("inode", 0644|fuse.S_IFREG, nil))
	assert.Empty(t, mapper.MapNodeID(1000), "Out of bounds NodeID should return no ID")
	assert.Empty(t, mapper.MapNodeID(0), "0 should be a nil NodeID")
}
