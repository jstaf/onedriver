package fs

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog/log"
)

const timeout = time.Second

func (f *Filesystem) getInodeContent(i *Inode) *[]byte {
	i.RLock()
	defer i.RUnlock()
	data := f.content.Get(i.DriveItem.ID)
	return &data
}

// remoteID uploads a file to obtain a Onedrive ID if it doesn't already
// have one. This is necessary to avoid race conditions against uploads if the
// file has not already been uploaded.
func (f *Filesystem) remoteID(i *Inode) (string, error) {
	if i.IsDir() {
		// Directories are always created with an ID. (And this method is only
		// really used for files anyways...)
		return i.ID(), nil
	}

	originalID := i.ID()
	if isLocalID(originalID) && f.auth.AccessToken != "" {
		// perform a blocking upload of the item
		data := f.getInodeContent(i)
		session, err := NewUploadSession(i, data)
		if err != nil {
			return originalID, err
		}

		i.Lock()
		name := i.DriveItem.Name
		err = session.Upload(f.auth)
		if err != nil {
			i.Unlock()

			if strings.Contains(err.Error(), "nameAlreadyExists") {
				// A file with this name already exists on the server, get its ID and
				// use that. This is probably the same file, but just got uploaded
				// earlier.
				children, err := graph.GetItemChildren(i.ParentID(), f.auth)
				if err != nil {
					return originalID, err
				}
				for _, child := range children {
					if child.Name == name {
						log.Info().
							Str("name", name).
							Str("originalID", originalID).
							Str("newID", child.ID).
							Msg("Exchanged ID.")
						return child.ID, f.MoveID(originalID, child.ID)
					}
				}
			}
			// failed to obtain an ID, return whatever it was beforehand
			return originalID, err
		}

		// we just successfully uploaded a copy, no need to do it again
		i.hasChanges = false
		i.DriveItem.ETag = session.ETag
		i.Unlock()

		// this is all we really wanted from this transaction
		err = f.MoveID(originalID, session.ID)
		log.Info().
			Str("name", name).
			Str("originalID", originalID).
			Str("newID", session.ID).
			Msg("Exchanged ID.")
		return session.ID, err
	}
	return originalID, nil
}

// Statfs returns information about the filesystem. Mainly useful for checking
// quotas and storage limits.
func (f *Filesystem) StatFs(cancel <-chan struct{}, in *fuse.InHeader, out *fuse.StatfsOut) fuse.Status {
	ctx := log.With().Str("op", "StatFs").Logger()
	ctx.Debug().Msg("")
	drive, err := graph.GetDrive(f.auth)
	if err != nil {
		return fuse.EREMOTEIO
	}

	if drive.DriveType == graph.DriveTypePersonal {
		ctx.Warn().Msg("Personal OneDrive accounts do not show number of files, " +
			"inode counts reported by onedriver will be bogus.")
	} else if drive.Quota.Total == 0 { // <-- check for if microsoft ever fixes their API
		ctx.Warn().Msg("OneDrive for Business accounts do not report quotas, " +
			"pretending the quota is 5TB and it's all unused.")
		drive.Quota.Total = 5 * uint64(math.Pow(1024, 4))
		drive.Quota.Remaining = 5 * uint64(math.Pow(1024, 4))
		drive.Quota.FileCount = 0
	}

	// limits are pasted from https://support.microsoft.com/en-us/help/3125202
	const blkSize uint64 = 4096 // default ext4 block size
	out.Bsize = uint32(blkSize)
	out.Blocks = drive.Quota.Total / blkSize
	out.Bfree = drive.Quota.Remaining / blkSize
	out.Bavail = drive.Quota.Remaining / blkSize
	out.Files = 100000
	out.Ffree = 100000 - drive.Quota.FileCount
	out.NameLen = 260
	return fuse.OK
}

// Mkdir creates a directory.
func (f *Filesystem) Mkdir(cancel <-chan struct{}, in *fuse.MkdirIn, name string, out *fuse.EntryOut) fuse.Status {
	inode := f.GetNodeID(in.NodeId)
	if inode == nil {
		return fuse.ENOENT
	}
	id := inode.ID()
	path := filepath.Join(inode.Path(), name)
	ctx := log.With().
		Str("op", "Mkdir").
		Uint64("nodeID", in.NodeId).
		Str("id", id).
		Str("path", path).
		Str("mode", Octal(in.Mode)).
		Logger()
	ctx.Debug().Msg("")

	// create the new directory on the server
	item, err := graph.Mkdir(name, id, f.auth)
	if err != nil {
		ctx.Error().Err(err).Msg("Could not create remote directory!")
		return fuse.EREMOTEIO
	}

	newInode := NewInodeDriveItem(item)
	newInode.mode = in.Mode | fuse.S_IFDIR

	out.NodeId = f.InsertChild(id, newInode)
	out.Attr = newInode.makeAttr()
	out.SetAttrTimeout(timeout)
	out.SetEntryTimeout(timeout)
	return fuse.OK
}

// Rmdir removes a directory if it's empty.
func (f *Filesystem) Rmdir(cancel <-chan struct{}, in *fuse.InHeader, name string) fuse.Status {
	parentID := f.TranslateID(in.NodeId)
	if parentID == "" {
		return fuse.ENOENT
	}
	child, _ := f.GetChild(parentID, name, f.auth)
	if child == nil {
		return fuse.ENOENT
	}
	if child.HasChildren() {
		return fuse.Status(syscall.ENOTEMPTY)
	}
	return f.Unlink(cancel, in, name)
}

// ReadDir provides a list of all the entries in the directory
func (f *Filesystem) OpenDir(cancel <-chan struct{}, in *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	id := f.TranslateID(in.NodeId)
	dir := f.GetID(id)
	if dir == nil {
		return fuse.ENOENT
	}
	if !dir.IsDir() {
		return fuse.ENOTDIR
	}
	path := dir.Path()
	ctx := log.With().
		Str("op", "OpenDir").
		Uint64("nodeID", in.NodeId).
		Str("id", id).
		Str("path", path).Logger()
	ctx.Debug().Msg("")

	children, err := f.GetChildrenID(id, f.auth)
	if err != nil {
		// not an item not found error (Lookup/Getattr will always be called
		// before Readdir()), something has happened to our connection
		ctx.Error().Err(err).Msg("Could not fetch children")
		return fuse.EREMOTEIO
	}

	parent := f.GetID(dir.ParentID())
	if parent == nil {
		// This is the parent of the mountpoint. The FUSE kernel module discards
		// this info, so what we put here doesn't actually matter.
		parent = NewInode("..", 0755|fuse.S_IFDIR, nil)
		parent.nodeID = math.MaxUint64
	}

	entries := make([]*Inode, 2)
	entries[0] = dir
	entries[1] = parent

	for _, child := range children {
		entries = append(entries, child)
	}
	f.opendirsM.Lock()
	f.opendirs[in.NodeId] = entries
	f.opendirsM.Unlock()

	return fuse.OK
}

// ReleaseDir closes a directory and purges it from memory
func (f *Filesystem) ReleaseDir(in *fuse.ReleaseIn) {
	f.opendirsM.Lock()
	delete(f.opendirs, in.NodeId)
	f.opendirsM.Unlock()
}

// ReadDirPlus reads an individual directory entry AND does a lookup.
func (f *Filesystem) ReadDirPlus(cancel <-chan struct{}, in *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	f.opendirsM.RLock()
	entries, ok := f.opendirs[in.NodeId]
	f.opendirsM.RUnlock()
	if !ok {
		// readdir can sometimes arrive before the corresponding opendir, so we force it
		f.OpenDir(cancel, &fuse.OpenIn{InHeader: in.InHeader}, nil)
		f.opendirsM.RLock()
		entries, ok = f.opendirs[in.NodeId]
		f.opendirsM.RUnlock()
		if !ok {
			return fuse.EBADF
		}
	}

	if in.Offset >= uint64(len(entries)) {
		// just tried to seek past end of directory, we're all done!
		return fuse.OK
	}

	inode := entries[in.Offset]
	entry := fuse.DirEntry{
		Ino:  inode.NodeID(),
		Mode: inode.Mode(),
	}
	// first two entries will always be "." and ".."
	switch in.Offset {
	case 0:
		entry.Name = "."
	case 1:
		entry.Name = ".."
	default:
		entry.Name = inode.Name()
	}
	entryOut := out.AddDirLookupEntry(entry)
	if entryOut == nil {
		//FIXME probably need to handle this better using the "overflow stuff"
		log.Error().
			Str("op", "ReadDirPlus").
			Uint64("nodeID", in.NodeId).
			Uint64("offset", in.Offset).
			Str("entryName", entry.Name).
			Uint64("entryNodeID", entry.Ino).
			Msg("Exceeded DirLookupEntry bounds!")
		return fuse.EIO
	}
	entryOut.NodeId = entry.Ino
	entryOut.Attr = inode.makeAttr()
	entryOut.SetAttrTimeout(timeout)
	entryOut.SetEntryTimeout(timeout)
	return fuse.OK
}

// ReadDir reads a directory entry. Usually doesn't get called (ReadDirPlus is
// typically used).
func (f *Filesystem) ReadDir(cancel <-chan struct{}, in *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	f.opendirsM.RLock()
	entries, ok := f.opendirs[in.NodeId]
	f.opendirsM.RUnlock()
	if !ok {
		// readdir can sometimes arrive before the corresponding opendir, so we force it
		f.OpenDir(cancel, &fuse.OpenIn{InHeader: in.InHeader}, nil)
		f.opendirsM.RLock()
		entries, ok = f.opendirs[in.NodeId]
		f.opendirsM.RUnlock()
		if !ok {
			return fuse.EBADF
		}
	}

	if in.Offset >= uint64(len(entries)) {
		// just tried to seek past end of directory, we're all done!
		return fuse.OK
	}

	inode := entries[in.Offset]
	entry := fuse.DirEntry{
		Ino:  inode.NodeID(),
		Mode: inode.Mode(),
	}
	// first two entries will always be "." and ".."
	switch in.Offset {
	case 0:
		entry.Name = "."
	case 1:
		entry.Name = ".."
	default:
		entry.Name = inode.Name()
	}

	out.AddDirEntry(entry)
	return fuse.OK
}

// Lookup is called by the kernel when the VFS wants to know about a file inside
// a directory.
func (f *Filesystem) Lookup(cancel <-chan struct{}, in *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	id := f.TranslateID(in.NodeId)
	log.Trace().
		Str("op", "Lookup").
		Uint64("nodeID", in.NodeId).
		Str("id", id).
		Str("name", name).
		Msg("")

	child, _ := f.GetChild(id, strings.ToLower(name), f.auth)
	if child == nil {
		return fuse.ENOENT
	}

	out.NodeId = child.NodeID()
	out.Attr = child.makeAttr()
	out.SetAttrTimeout(timeout)
	out.SetEntryTimeout(timeout)
	return fuse.OK
}

// Mknod creates a regular file. The server doesn't have this yet.
func (f *Filesystem) Mknod(cancel <-chan struct{}, in *fuse.MknodIn, name string, out *fuse.EntryOut) fuse.Status {
	parentID := f.TranslateID(in.NodeId)
	if parentID == "" {
		return fuse.EBADF
	}

	parent := f.GetID(parentID)
	if parent == nil {
		return fuse.ENOENT
	}

	path := filepath.Join(parent.Path(), name)
	ctx := log.With().
		Str("op", "Mknod").
		Uint64("nodeID", in.NodeId).
		Str("path", path).
		Logger()
	if f.IsOffline() {
		ctx.Warn().Msg("We are offline. Refusing Mknod() to avoid data loss later.")
		return fuse.EROFS
	}

	if child, _ := f.GetChild(parentID, name, f.auth); child != nil {
		return fuse.Status(syscall.EEXIST)
	}

	inode := NewInode(name, in.Mode, parent)
	ctx.Debug().
		Str("childID", inode.ID()).
		Str("mode", Octal(in.Mode)).
		Msg("Creating inode.")
	out.NodeId = f.InsertChild(parentID, inode)
	out.Attr = inode.makeAttr()
	out.SetAttrTimeout(timeout)
	out.SetEntryTimeout(timeout)
	return fuse.OK
}

// Create creates a regular file and opens it. The server doesn't have this yet.
func (f *Filesystem) Create(cancel <-chan struct{}, in *fuse.CreateIn, name string, out *fuse.CreateOut) fuse.Status {
	// we reuse mknod here
	result := f.Mknod(
		cancel,
		// we don't actually use the umask or padding here, so they don't get passed
		&fuse.MknodIn{
			InHeader: in.InHeader,
			Mode:     in.Mode,
		},
		name,
		&out.EntryOut,
	)
	if result == fuse.Status(syscall.EEXIST) {
		// if the inode already exists, we should truncate the existing file and
		// return the existing file inode as per "man creat"
		parentID := f.TranslateID(in.NodeId)
		child, _ := f.GetChild(parentID, name, f.auth)
		log.Debug().
			Str("op", "Create").
			Uint64("nodeID", in.NodeId).
			Str("id", parentID).
			Str("childID", child.ID()).
			Str("path", child.Path()).
			Str("mode", Octal(in.Mode)).
			Msg("Child inode already exists, truncating.")
		f.content.Delete(child.ID())
		f.content.Open(child.ID())
		child.DriveItem.Size = 0
		child.hasChanges = true
		return fuse.OK
	}
	// no further initialized required to open the file, it's empty
	return result
}

// Open fetches a Inodes's content and initializes the .Data field with actual
// data from the server.
func (f *Filesystem) Open(cancel <-chan struct{}, in *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	id := f.TranslateID(in.NodeId)
	inode := f.GetID(id)
	if inode == nil {
		return fuse.ENOENT
	}

	path := inode.Path()
	ctx := log.With().
		Str("op", "Open").
		Uint64("nodeID", in.NodeId).
		Str("id", id).
		Str("path", path).
		Logger()

	flags := int(in.Flags)
	if flags&os.O_RDWR+flags&os.O_WRONLY > 0 && f.IsOffline() {
		ctx.Warn().
			Bool("readWrite", flags&os.O_RDWR > 0).
			Bool("writeOnly", flags&os.O_WRONLY > 0).
			Msg("Refusing Open() with write flag, FS is offline.")
		return fuse.EROFS
	}

	ctx.Debug().Msg("")

	// try grabbing from disk
	fd, err := f.content.Open(id)
	if err != nil {
		ctx.Error().Err(err).Msg("Could not create cache file.")
		return fuse.EIO
	}

	if isLocalID(id) {
		// just use whatever's present if we're the only ones who have it
		return fuse.OK
	}

	// we have something on disk-
	// verify content against what we're supposed to have
	inode.Lock()
	defer inode.Unlock()
	// stay locked until end to prevent multiple Opens() from competing for
	// downloads of the same file.

	var hashMatch bool
	driveType := inode.DriveItem.Parent.DriveType
	if driveType == graph.DriveTypePersonal {
		hashMatch = inode.VerifyChecksum(graph.SHA1HashStream(fd))
	} else if driveType == graph.DriveTypeBusiness || driveType == graph.DriveTypeSharepoint {
		hashMatch = inode.VerifyChecksum(graph.QuickXORHashStream(fd))
	} else {
		hashMatch = true
		ctx.Warn().Str("driveType", driveType).
			Msg("Could not determine drive type, not checking hashes.")
	}

	if hashMatch {
		// disk content is only used if the checksums match
		ctx.Info().Msg("Found content in cache.")

		// we check size ourselves in case the API file sizes are WRONG (it happens)
		st, _ := fd.Stat()
		inode.DriveItem.Size = uint64(st.Size())
		return fuse.OK
	}
	ctx.Info().Str("drivetype", driveType).
		Msg("Not using cached item due to file hash mismatch.")

	// didn't have it on disk, now try api
	ctx.Info().Msg("Fetching remote content for item from API.")
	size, err := graph.GetItemContentStream(id, f.auth, fd)
	if err != nil {
		ctx.Error().Err(err).Msg("Failed to fetch remote content.")
		return fuse.EREMOTEIO
	}
	inode.DriveItem.Size = size
	return fuse.OK
}

// Unlink deletes a child file.
func (f *Filesystem) Unlink(cancel <-chan struct{}, in *fuse.InHeader, name string) fuse.Status {
	parentID := f.TranslateID(in.NodeId)
	child, _ := f.GetChild(parentID, name, nil)
	if child == nil {
		// the file we are unlinking never existed
		return fuse.ENOENT
	}
	if f.IsOffline() {
		return fuse.EROFS
	}

	id := child.ID()
	path := child.Path()
	ctx := log.With().
		Str("op", "Unlink").
		Uint64("nodeID", in.NodeId).
		Str("id", parentID).
		Str("childID", id).
		Str("path", path).
		Logger()
	ctx.Debug().Msg("Unlinking inode.")

	// if no ID, the item is local-only, and does not need to be deleted on the
	// server
	if !isLocalID(id) {
		if err := graph.Remove(id, f.auth); err != nil {
			ctx.Err(err).Msg("Failed to delete item on server. Aborting op.")
			return fuse.EREMOTEIO
		}
	}

	f.DeleteID(id)
	f.content.Delete(id)
	return fuse.OK
}

// Read an inode's data like a file.
func (f *Filesystem) Read(cancel <-chan struct{}, in *fuse.ReadIn, buf []byte) (fuse.ReadResult, fuse.Status) {
	inode := f.GetNodeID(in.NodeId)
	if inode == nil {
		return fuse.ReadResultData(make([]byte, 0)), fuse.EBADF
	}

	id := inode.ID()
	path := inode.Path()
	ctx := log.With().
		Str("op", "Read").
		Uint64("nodeID", in.NodeId).
		Str("id", id).
		Str("path", path).
		Int("bufsize", len(buf)).
		Logger()
	ctx.Trace().Msg("")

	fd, err := f.content.Open(id)
	if err != nil {
		ctx.Error().Err(err).Msg("Cache Open() failed.")
		return fuse.ReadResultData(make([]byte, 0)), fuse.EIO
	}

	// we are locked for the remainder of this op
	inode.RLock()
	defer inode.RUnlock()
	return fuse.ReadResultFd(fd.Fd(), int64(in.Offset), int(in.Size)), fuse.OK
}

// Write to an Inode like a file. Note that changes are 100% local until
// Flush() is called. Returns the number of bytes written and the status of the
// op.
func (f *Filesystem) Write(cancel <-chan struct{}, in *fuse.WriteIn, data []byte) (uint32, fuse.Status) {
	id := f.TranslateID(in.NodeId)
	inode := f.GetID(id)
	if inode == nil {
		return 0, fuse.EBADF
	}

	nWrite := len(data)
	offset := int(in.Offset)
	ctx := log.With().
		Str("op", "Write").
		Str("id", id).
		Uint64("nodeID", in.NodeId).
		Str("path", inode.Path()).
		Int("bufsize", nWrite).
		Int("offset", offset).
		Logger()
	ctx.Trace().Msg("")

	fd, err := f.content.Open(id)
	if err != nil {
		ctx.Error().Msg("Cache Open() failed.")
		return 0, fuse.EIO
	}

	inode.Lock()
	defer inode.Unlock()
	n, err := fd.WriteAt(data, int64(offset))
	if err != nil {
		ctx.Error().Err(err).Msg("Error during write")
		return uint32(n), fuse.EIO
	}

	st, _ := fd.Stat()
	inode.DriveItem.Size = uint64(st.Size())
	inode.hasChanges = true
	return uint32(n), fuse.OK
}

// Fsync is a signal to ensure writes to the Inode are flushed to stable
// storage. This method is used to trigger uploads of file content.
func (f *Filesystem) Fsync(cancel <-chan struct{}, in *fuse.FsyncIn) fuse.Status {
	id := f.TranslateID(in.NodeId)
	inode := f.GetID(id)
	if inode == nil {
		return fuse.EBADF
	}

	ctx := log.With().
		Str("op", "Fsync").
		Str("id", id).
		Uint64("nodeID", in.NodeId).
		Str("path", inode.Path()).
		Logger()
	ctx.Debug().Msg("")
	if inode.HasChanges() {
		inode.Lock()
		inode.hasChanges = false

		// recompute hashes when saving new content
		inode.DriveItem.File = &graph.File{}
		fd, err := f.content.Open(id)
		if err != nil {
			ctx.Error().Err(err).Msg("Could not get fd.")
		}
		fd.Sync()
		if inode.DriveItem.Parent.DriveType == graph.DriveTypePersonal {
			inode.DriveItem.File.Hashes.SHA1Hash = graph.SHA1HashStream(fd)
		} else {
			inode.DriveItem.File.Hashes.QuickXorHash = graph.QuickXORHashStream(fd)
		}
		inode.Unlock()

		if err := f.uploads.QueueUpload(inode); err != nil {
			ctx.Error().Err(err).Msg("Error creating upload session.")
			return fuse.EREMOTEIO
		}
		return fuse.OK
	}
	return fuse.OK
}

// Flush is called when a file descriptor is closed. Uses Fsync() to perform file
// uploads. (Release not implemented because all cleanup is already done here).
func (f *Filesystem) Flush(cancel <-chan struct{}, in *fuse.FlushIn) fuse.Status {
	inode := f.GetNodeID(in.NodeId)
	if inode == nil {
		return fuse.EBADF
	}

	id := inode.ID()
	log.Trace().
		Str("op", "Flush").
		Str("id", id).
		Str("path", inode.Path()).
		Uint64("nodeID", in.NodeId).
		Msg("")
	f.Fsync(cancel, &fuse.FsyncIn{InHeader: in.InHeader})
	f.content.Close(id)
	return 0
}

// Getattr returns a the Inode as a UNIX stat. Holds the read mutex for all of
// the "metadata fetch" operations.
func (f *Filesystem) GetAttr(cancel <-chan struct{}, in *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	id := f.TranslateID(in.NodeId)
	inode := f.GetID(id)
	if inode == nil {
		return fuse.ENOENT
	}
	log.Trace().
		Str("op", "GetAttr").
		Uint64("nodeID", in.NodeId).
		Str("id", id).
		Str("path", inode.Path()).
		Msg("")

	out.Attr = inode.makeAttr()
	out.SetTimeout(timeout)
	return fuse.OK
}

// Setattr is the workhorse for setting filesystem attributes. Does the work of
// operations like utimens, chmod, chown (not implemented, FUSE is single-user),
// and truncate.
func (f *Filesystem) SetAttr(cancel <-chan struct{}, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	i := f.GetNodeID(in.NodeId)
	if i == nil {
		return fuse.ENOENT
	}
	path := i.Path()
	isDir := i.IsDir() // holds an rlock
	i.Lock()

	ctx := log.With().
		Str("op", "SetAttr").
		Uint64("nodeID", in.NodeId).
		Str("id", i.DriveItem.ID).
		Str("path", path).
		Logger()

	// utimens
	if mtime, valid := in.GetMTime(); valid {
		ctx.Info().
			Str("subop", "utimens").
			Time("oldMtime", *i.DriveItem.ModTime).
			Time("newMtime", *i.DriveItem.ModTime).
			Msg("")
		i.DriveItem.ModTime = &mtime
	}

	// chmod
	if mode, valid := in.GetMode(); valid {
		ctx.Info().
			Str("subop", "chmod").
			Str("oldMode", Octal(i.mode)).
			Str("newMode", Octal(mode)).
			Msg("")
		if isDir {
			i.mode = fuse.S_IFDIR | mode
		} else {
			i.mode = fuse.S_IFREG | mode
		}
	}

	// truncate
	if size, valid := in.GetSize(); valid {
		ctx.Info().
			Str("subop", "truncate").
			Uint64("oldSize", i.DriveItem.Size).
			Uint64("newSize", size).
			Msg("")
		fd, _ := f.content.Open(i.DriveItem.ID)
		fd.Truncate(int64(size))
		i.DriveItem.Size = size
		i.hasChanges = true
	}

	i.Unlock()
	out.Attr = i.makeAttr()
	out.SetTimeout(timeout)
	return fuse.OK
}

// Rename renames and/or moves an inode.
func (f *Filesystem) Rename(cancel <-chan struct{}, in *fuse.RenameIn, name string, newName string) fuse.Status {
	oldParentID := f.TranslateID(in.NodeId)
	oldParentItem := f.GetNodeID(in.NodeId)
	if oldParentID == "" || oldParentItem == nil {
		return fuse.EBADF
	}
	path := filepath.Join(oldParentItem.Path(), name)

	// we'll have the metadata for the dest inode already so it is not necessary
	// to use GetPath() to prefetch it. In order for the fs to know about this
	// inode, it has already fetched all of the inodes up to the new destination.
	newParentItem := f.GetNodeID(in.Newdir)
	if newParentItem == nil {
		return fuse.ENOENT
	}
	dest := filepath.Join(newParentItem.Path(), newName)

	inode, _ := f.GetChild(oldParentID, name, f.auth)
	id, err := f.remoteID(inode)
	newParentID := newParentItem.ID()

	ctx := log.With().
		Str("op", "Rename").
		Str("id", id).
		Str("parentID", newParentID).
		Str("path", path).
		Str("dest", dest).
		Logger()
	ctx.Info().
		Uint64("srcNodeID", in.NodeId).
		Uint64("dstNodeID", in.Newdir).
		Msg("")

	if isLocalID(id) || err != nil {
		// uploads will fail without an id
		ctx.Error().Err(err).
			Msg("ID of item to move cannot be local and we failed to obtain an ID.")
		return fuse.EREMOTEIO
	}

	// perform remote rename
	if err = graph.Rename(id, newName, newParentID, f.auth); err != nil {
		ctx.Error().Err(err).Msg("Failed to rename remote item.")
		return fuse.EREMOTEIO
	}

	// now rename local copy
	if err = f.MovePath(oldParentID, newParentID, name, newName, f.auth); err != nil {
		ctx.Error().Err(err).Msg("Failed to rename local item.")
		return fuse.EIO
	}

	// whew! item renamed
	return fuse.OK
}
