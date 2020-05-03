package fs

import (
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	log "github.com/sirupsen/logrus"
)

// UploadManager is used to manage and retry uploads.
type UploadManager struct {
	queue         chan *UploadSession
	deletionQueue chan string
	sessions      map[string]*UploadSession
	auth          *graph.Auth
}

// NewUploadManager creates a new queue/thread for uploads
func NewUploadManager(duration time.Duration, auth *graph.Auth) *UploadManager {
	manager := UploadManager{
		queue:         make(chan *UploadSession),
		deletionQueue: make(chan string),
		sessions:      make(map[string]*UploadSession),
		auth:          auth,
	}
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
			u.sessions[session.ID] = session

		case cancelID := <-u.deletionQueue: // remove uploads for deleted items
			session, exists := u.sessions[cancelID]
			if exists {
				session.cancel(u.auth)
				delete(u.sessions, cancelID)
			}

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
						session.cancel(u.auth)
						delete(u.sessions, session.ID)
					}

					log.WithFields(log.Fields{
						"id":  session.ID,
						"err": session.Error(),
					}).Warning("Upload session failed, will retry from beginning.")
					session.cancel(u.auth) // cancel large sessions
					session.setState(notStarted, nil)
				case complete:
					delete(u.sessions, session.ID)
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
