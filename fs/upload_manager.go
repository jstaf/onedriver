package fs

import (
	"encoding/json"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

// UPLOADS is just a constant to avoid typos in bucket names when referenced
// elsewhere
var UPLOADS = []byte("uploads")

// UploadManager is used to manage and retry uploads.
type UploadManager struct {
	queue         chan *UploadSession
	deletionQueue chan string
	sessions      map[string]*UploadSession
	auth          *graph.Auth
	db            *bolt.DB
}

// NewUploadManager creates a new queue/thread for uploads
func NewUploadManager(duration time.Duration, db *bolt.DB, auth *graph.Auth) *UploadManager {
	manager := UploadManager{
		queue:         make(chan *UploadSession),
		deletionQueue: make(chan string),
		sessions:      make(map[string]*UploadSession),
		auth:          auth,
		db:            db,
	}
	db.View(func(tx *bolt.Tx) error {
		// Add any incomplete sessions from disk - any sessions here were never
		// finished. The most likely cause of this is that the user shut off
		// their computer or closed the program after starting the upload.
		b := tx.Bucket(UPLOADS)
		if b == nil {
			// bucket does not exist yet, bail out early
			return nil
		}
		return b.ForEach(func(key []byte, val []byte) error {
			session := &UploadSession{}
			err := json.Unmarshal(val, session)
			if err != nil {
				return err
			}
			session.cancel(auth) // uploads are currently non-resumable
			manager.sessions[session.ID] = session
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
			u.db.Update(func(tx *bolt.Tx) error {
				// persist to disk in case the user shuts off their computer or
				// kills onedriver prematurely
				contents, _ := json.Marshal(session)
				b, _ := tx.CreateBucketIfNotExists(UPLOADS)
				return b.Put([]byte(session.ID), contents)
			})
			u.sessions[session.ID] = session

		case cancelID := <-u.deletionQueue: // remove uploads for deleted items
			u.finishUpload(cancelID)

		case <-ticker.C: // periodically start uploads, or remove them if done/failed
			for _, session := range u.sessions {
				switch session.getState() {
				case notStarted:
					go session.Upload(u.auth)

				case errored:
					session.retries++
					if session.retries > 5 {
						log.WithFields(log.Fields{
							"id":      session.ID,
							"err":     session.Error(),
							"retries": session.retries,
						}).Error(
							"Upload session failed too many times, cancelling session. " +
								"This is a bug - please file a bug report!",
						)
						u.finishUpload(session.ID)
					}

					log.WithFields(log.Fields{
						"id":  session.ID,
						"err": session.Error(),
					}).Warning("Upload session failed, will retry from beginning.")
					session.cancel(u.auth) // cancel large sessions
					session.setState(notStarted, nil)

				case complete:
					u.finishUpload(session.ID)
				}
			}
		}
	}
}

// QueueUpload queues an item for upload.
func (u *UploadManager) QueueUpload(inode *Inode) error {
	session, err := NewUploadSession(inode, u.auth)
	if err == nil {
		u.queue <- session
	}
	return err
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
	u.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(UPLOADS).Delete([]byte(id))
	})
	delete(u.sessions, id)
}
