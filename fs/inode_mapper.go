package fs

import "sync"

// InodeMapper maps numeric kernel IDs to and from Microsoft Graph IDs.
type InodeMapper struct {
	sync.RWMutex
	lastNodeID uint64 // 1-based... 0 is null value
	inodes     []string
}

// MapID returns the DriveItemID for a given NodeID
func (i *InodeMapper) MapNodeID(nodeID uint64) string {
	if nodeID == 0 {
		return ""
	}

	i.RLock()
	defer i.RUnlock()
	if nodeID > i.lastNodeID {
		return ""
	}
	return i.inodes[nodeID-1]
}

// AssignNodeID assigns a numeric inode ID used by the kernel if one is not
// already assigned. Will safely reassign the NodeID if the inode's NodeID is
// wrong for some reason.
func (i *InodeMapper) AssignNodeID(inode *Inode) uint64 {
	inode.RLock()
	nodeID := inode.nodeID
	id := inode.DriveItem.ID
	inode.RUnlock()

	// if the existing id was unset or doesn't pull out the right inode, reassign
	if i.MapNodeID(nodeID) == id {
		return nodeID
	}

	// lock ordering is to satisfy deadlock detector
	inode.Lock()
	defer inode.Unlock()
	i.Lock()
	defer i.Unlock()

	i.lastNodeID++
	i.inodes = append(i.inodes, inode.DriveItem.ID)
	inode.nodeID = i.lastNodeID
	return i.lastNodeID
}
