package graph

import (
	"time"

	log "github.com/sirupsen/logrus"
)

// UploadManager is used to manage and retry uploads.
type UploadManager struct {
	queue    chan *UploadSession
	sessions map[string]*UploadSession
	auth     *Auth
}

// NewUploadManager creates a new queue/thread for uploads
func NewUploadManager(duration time.Duration, auth *Auth) *UploadManager {
	manager := UploadManager{
		queue:    make(chan *UploadSession),
		sessions: make(map[string]*UploadSession),
		auth:     auth,
	}
	go manager.uploadLoop(duration)
	return &manager
}

// uploadLoop manages the deduplication and tracking of uploads
func (u *UploadManager) uploadLoop(duration time.Duration) {
	ticker := time.NewTicker(duration)
	for {
		select {
		case session := <-u.queue:
			// deduplicate sessions for the same item
			if old, exists := u.sessions[session.ID]; exists {
				old.cancel(u.auth)
			}
			u.sessions[session.ID] = session
		case <-ticker.C:
			// periodically start uploads, or remove them if done/failed
			for _, session := range u.sessions {
				switch session.getState() {
				case notStarted:
					go session.Upload(u.auth)
				case errored:
					log.WithField("id", session.ID).Error("Upload failed.")
					fallthrough
				case complete:
					delete(u.sessions, session.ID)
				}
			}
		}
	}
}

// QueueUpload queues an item for upload.
func (u *UploadManager) QueueUpload(item *DriveItem) error {
	session, err := NewUploadSession(item, u.auth)
	if err == nil {
		u.queue <- session
	}
	return err
}
