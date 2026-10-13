package fs

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog/log"
	bolt "go.etcd.io/bbolt"
)

const maxUploadsInFlight = 5

// number of content hashes remembered per item for conflict detection
const maxUploadHashes = 10

var bucketUploads = []byte("uploads")

// UploadManager is used to manage and retry uploads.
type UploadManager struct {
	queue         chan *UploadSession
	deletionQueue chan string
	sessions      map[string]*UploadSession
	inFlight      uint8 // number of sessions in flight
	auth          *graph.Auth
	fs            *Filesystem
	db            *bolt.DB

	// sessionsM guards writes to sessions (which only happen on the
	// uploadLoop goroutine) against reads from other goroutines (the delta
	// loop), as well as reserved and hashes. It is never held while acquiring
	// an inode lock.
	sessionsM sync.RWMutex
	reserved  map[string]int      // uploads queued but not yet received by uploadLoop
	hashes    map[string][]string // recent content hashes we have uploaded, by ID
}

// NewUploadManager creates a new queue/thread for uploads
func NewUploadManager(duration time.Duration, db *bolt.DB, fs *Filesystem, auth *graph.Auth) *UploadManager {
	manager := UploadManager{
		queue:         make(chan *UploadSession),
		deletionQueue: make(chan string, 1000), // FIXME - why does this chan need to be buffered now???
		sessions:      make(map[string]*UploadSession),
		reserved:      make(map[string]int),
		hashes:        make(map[string][]string),
		auth:          auth,
		db:            db,
		fs:            fs,
	}
	db.View(func(tx *bolt.Tx) error {
		// Add any incomplete sessions from disk - any sessions here were never
		// finished. The most likely cause of this is that the user shut off
		// their computer or closed the program after starting the upload.
		b := tx.Bucket(bucketUploads)
		if b == nil {
			// bucket does not exist yet, bail out early
			return nil
		}
		return b.ForEach(func(key []byte, val []byte) error {
			session := &UploadSession{}
			err := json.Unmarshal(val, session)
			if err != nil {
				log.Error().Err(err).Msg("Failure restoring upload sessions from disk.")
				return err
			}
			if session.getState() != uploadNotStarted {
				manager.inFlight++
			}
			session.cancel(auth) // uploads are currently non-resumable
			manager.sessions[session.ID] = session
			manager.recordHashLocked(session.ID, session.QuickXORHash)
			return nil
		})
	})
	go manager.uploadLoop(duration)
	return &manager
}

// uploadLoop manages the deduplication and tracking of uploads
func (u *UploadManager) uploadLoop(duration time.Duration) {
	ticker := time.NewTicker(duration)
	for {
		select {
		case session := <-u.queue: // new sessions
			// deduplicate sessions for the same item
			if old, exists := u.sessions[session.ID]; exists {
				old.cancel(u.auth)
			}
			contents, _ := json.Marshal(session)
			u.db.Batch(func(tx *bolt.Tx) error {
				// persist to disk in case the user shuts off their computer or
				// kills onedriver prematurely
				b, _ := tx.CreateBucketIfNotExists(bucketUploads)
				return b.Put([]byte(session.ID), contents)
			})
			// swap the reservation for the real session atomically so the
			// delta loop never sees this upload as "not pending"
			u.sessionsM.Lock()
			u.sessions[session.ID] = session
			u.releaseLocked(session.reservedID)
			u.recordHashLocked(session.ID, session.QuickXORHash)
			u.sessionsM.Unlock()

		case cancelID := <-u.deletionQueue: // remove uploads for deleted items
			u.finishUpload(cancelID)

		case <-ticker.C: // periodically start uploads, or remove them if done/failed
			for _, session := range u.sessions {
				switch session.getState() {
				case uploadNotStarted:
					// max active upload sessions are capped at this limit for faster
					// uploads of individual files and also to prevent possible server-
					// side throttling that can cause errors.
					if u.inFlight < maxUploadsInFlight {
						u.inFlight++
						go session.Upload(u.auth)
					}

				case uploadErrored:
					session.retries++
					if session.retries > 5 {
						log.Error().
							Str("id", session.ID).
							Str("name", session.Name).
							Err(session).
							Int("retries", session.retries).
							Msg("Upload session failed too many times, cancelling session.")
						u.finishUpload(session.ID)
					}

					log.Warn().
						Str("id", session.ID).
						Str("name", session.Name).
						Err(session).
						Msg("Upload session failed, will retry from beginning.")
					session.cancel(u.auth) // cancel large sessions
					session.setState(uploadNotStarted, nil)

				case uploadComplete:
					log.Info().
						Str("id", session.ID).
						Str("oldID", session.OldID).
						Str("name", session.Name).
						Msg("Upload completed!")

					// ID changed during upload, move to new ID
					if session.OldID != session.ID {
						u.recordHash(session.ID, session.QuickXORHash)
						err := u.fs.MoveID(session.OldID, session.ID)
						if err != nil {
							log.Error().
								Str("id", session.ID).
								Str("oldID", session.OldID).
								Str("name", session.Name).
								Err(err).
								Msg("Could not move inode to new ID!")
						}
					}

					// inode will exist at the new ID now, but we check if inode
					// is nil to see if the item has been deleted since upload start
					if inode := u.fs.GetID(session.ID); inode != nil {
						inode.Lock()
						inode.DriveItem.ETag = session.ETag
						inode.Unlock()
					}

					// the old ID is the one that was used to add it to the queue.
					// cleanup the session.
					u.finishUpload(session.OldID)
				}
			}
		}
	}
}

// QueueUpload queues an item for upload.
func (u *UploadManager) QueueUpload(inode *Inode) error {
	id := inode.ID()
	u.reserveUpload(id)
	return u.queueReservedUpload(inode, id)
}

// reserveUpload marks an upload for an ID as pending before the session is
// actually created. Callers that clear an inode's hasChanges flag must reserve
// first, so that hasChanges and IsUploadPending never both report false while
// local content is still waiting to be uploaded. Must be paired with
// queueReservedUpload.
func (u *UploadManager) reserveUpload(id string) {
	u.sessionsM.Lock()
	u.reserved[id]++
	u.sessionsM.Unlock()
}

// queueReservedUpload queues an upload previously reserved under reservedID.
func (u *UploadManager) queueReservedUpload(inode *Inode, reservedID string) error {
	data := u.fs.getInodeContent(inode)
	session, err := NewUploadSession(inode, data)
	if err != nil {
		u.sessionsM.Lock()
		u.releaseLocked(reservedID)
		u.sessionsM.Unlock()
		return err
	}
	session.reservedID = reservedID
	u.queue <- session
	return nil
}

// releaseLocked drops a reservation. sessionsM must be held.
func (u *UploadManager) releaseLocked(id string) {
	if u.reserved[id] <= 1 {
		delete(u.reserved, id)
	} else {
		u.reserved[id]--
	}
}

// recordHash remembers that we uploaded content with this hash for an ID.
func (u *UploadManager) recordHash(id string, hash string) {
	u.sessionsM.Lock()
	u.recordHashLocked(id, hash)
	u.sessionsM.Unlock()
}

// recordHashLocked is recordHash, but sessionsM must be held.
func (u *UploadManager) recordHashLocked(id string, hash string) {
	if hash == "" {
		return
	}
	hashes := u.hashes[id]
	for _, h := range hashes {
		if h == hash {
			return
		}
	}
	hashes = append(hashes, hash)
	if len(hashes) > maxUploadHashes {
		hashes = hashes[len(hashes)-maxUploadHashes:]
	}
	u.hashes[id] = hashes
}

// IsUploadPending returns true if an upload for any of the given IDs is
// queued, in progress, or waiting to be retried. Safe to call from any
// goroutine.
func (u *UploadManager) IsUploadPending(ids ...string) bool {
	u.sessionsM.RLock()
	defer u.sessionsM.RUnlock()
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, exists := u.sessions[id]; exists || u.reserved[id] > 0 {
			return true
		}
	}
	return false
}

// UploadedHashes returns the content hashes this client has recently uploaded
// for any of the given IDs. Safe to call from any goroutine.
func (u *UploadManager) UploadedHashes(ids ...string) []string {
	u.sessionsM.RLock()
	defer u.sessionsM.RUnlock()
	out := make([]string, 0)
	for _, id := range ids {
		out = append(out, u.hashes[id]...)
	}
	return out
}

// CancelUpload is used to kill any pending uploads for a session
func (u *UploadManager) CancelUpload(id string) {
	u.deletionQueue <- id
}

// finishUpload is an internal method that gets called when a session is
// completed. It cancels the session if one was in progress, and then deletes
// it from both memory and disk.
func (u *UploadManager) finishUpload(id string) {
	if session, exists := u.sessions[id]; exists {
		session.cancel(u.auth)
	}
	u.db.Batch(func(tx *bolt.Tx) error {
		if b := tx.Bucket(bucketUploads); b != nil {
			b.Delete([]byte(id))
		}
		return nil
	})
	if u.inFlight > 0 {
		u.inFlight--
	}
	u.sessionsM.Lock()
	delete(u.sessions, id)
	u.sessionsM.Unlock()
}
