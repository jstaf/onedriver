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
// already assigned. Will reassign node
func (i *InodeMapper) AssignNodeID(inode *Inode) uint64 {
	inode.RLock()
	nodeID := inode.nodeID
	id := inode.DriveItem.ID
	inode.RUnlock()

	if nodeID == 0 {
		// existing nodeID was unset, assign one

		// lock ordering is to satisfy deadlock detector
		inode.Lock()
		defer inode.Unlock()
		i.Lock()
		defer i.Unlock()

		i.lastNodeID++
		i.inodes = append(i.inodes, inode.DriveItem.ID)
		inode.nodeID = i.lastNodeID
		return i.lastNodeID
	} else if oldID := i.MapNodeID(nodeID); oldID != id && isLocalID(oldID) {
		// old ID has changed from a local id
		// update the list of inodes to reflect the new ID
		i.Lock()
		i.inodes[nodeID-1] = id
		i.Unlock()
	}
	// return existing node id
	return nodeID
}
