package graph

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	mu "github.com/sasha-s/go-deadlock"
	log "github.com/sirupsen/logrus"
)

// 10MB is the recommended upload size according to the graph API docs
const chunkSize uint64 = 10 * 1024 * 1024

// upload states
const (
	notStarted = iota
	started
	complete
	errored
)

// UploadSession contains a snapshot of the file we're uploading. We have to
// take the snapshot or the file may have changed on disk during upload (which
// would break the upload).
type UploadSession struct {
	ID                 string    `json:"id"`
	UploadURL          string    `json:"uploadUrl"`
	ExpirationDateTime time.Time `json:"expirationDateTime"`
	Size               uint64    `json:"-"`
	data               []byte

	mutex mu.Mutex
	state int
}

// UploadSessionPost is the initial post used to create an upload session
type UploadSessionPost struct {
	Name             string `json:"name,omitempty"`
	ConflictBehavior string `json:"@microsoft.graph.conflictBehavior,omitempty"`
	FileSystemInfo   `json:"fileSystemInfo,omitempty"`
}

// FileSystemInfo carries the filesystem metadata like Mtime/Atime
type FileSystemInfo struct {
	CreatedDateTime      time.Time `json:"createdDateTime,omitempty"`
	LastAccessedDateTime time.Time `json:"lastAccessedDateTime,omitempty"`
	LastModifiedDateTime time.Time `json:"lastModifiedDateTime,omitempty"`
}

// isLargeSession returns whether or not this is a formal upload session that
// must be registered with the API (over 4MB, according to the documentation).
func (u *UploadSession) isLargeSession() bool {
	return u.Size > 4*1024*1024
}

func (u *UploadSession) getState() int {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	return u.state
}

func (u *UploadSession) setState(state int) {
	u.mutex.Lock()
	u.state = state
	u.mutex.Unlock()
}

// NewUploadSession wraps an upload of a file into an UploadSession struct
// responsible for performing uploads for a file.
func NewUploadSession(d *DriveItem, auth *Auth) (*UploadSession, error) {
	id, err := d.RemoteID(auth)
	if err != nil || isLocalID(id) {
		log.WithFields(log.Fields{
			"err":  err,
			"path": d.Path(),
		}).Errorf("Could not obtain remote ID for upload.")
		return nil, err
	}

	d.mutex.RLock()
	// create a generic session for all files
	session := UploadSession{
		ID:   d.IDInternal,
		Size: d.SizeInternal,
		data: make([]byte, d.SizeInternal),
	}
	if d.data == nil {
		log.WithFields(log.Fields{
			"id":   d.IDInternal,
			"name": d.NameInternal,
		}).Error("Tried to dereference a nil pointer.")
		defer d.mutex.RUnlock()
		return nil, errors.New("inode data was nil")
	}
	copy(session.data, *d.data)
	d.mutex.RUnlock()

	if session.isLargeSession() {
		// must create a formal upload session with the API
		sessionResp, _ := json.Marshal(UploadSessionPost{
			ConflictBehavior: "replace",
			FileSystemInfo: FileSystemInfo{
				LastModifiedDateTime: time.Unix(int64(d.ModTime()), 0),
			},
		})

		resp, err := Post(
			fmt.Sprintf("/me/drive/items/%s/createUploadSession", session.ID),
			auth,
			bytes.NewReader(sessionResp),
		)
		if err != nil {
			return nil, err
		}

		// populates UploadURL/expiration
		if err = json.Unmarshal(resp, &session); err != nil {
			return nil, err
		}
	}
	return &session, nil
}

// cancel the upload session by deleting the temp file at the endpoint.
func (u *UploadSession) cancel(auth *Auth) {
	// is it an actual API upload session?
	if u.isLargeSession() {
		// dont care about result, this is purely us being polite to the server
		go Delete(u.UploadURL, auth)
	}
}

// Internal method used for uploading individual chunks of a DriveItem. We have
// to make things this way because the internal Put func doesn't work all that
// well when we need to add custom headers.
func (u *UploadSession) uploadChunk(auth *Auth, offset uint64) ([]byte, int, error) {
	if u.UploadURL == "" {
		return nil, -1, errors.New("uploadSession UploadURL cannot be empty")
	}

	// how much of the file are we going to upload?
	end := offset + chunkSize
	var reqChunkSize uint64
	if end > u.Size {
		end = u.Size
		reqChunkSize = end - offset + 1
	}
	if offset > u.Size {
		return nil, -1, errors.New("offset cannot be larger than DriveItem size")
	}

	auth.Refresh()

	client := &http.Client{}
	request, _ := http.NewRequest(
		"PUT",
		u.UploadURL,
		bytes.NewReader((u.data)[offset:end]),
	)
	// no Authorization header - it will throw a 401 if present
	request.Header.Add("Content-Length", strconv.Itoa(int(reqChunkSize)))
	frags := fmt.Sprintf("bytes %d-%d/%d", offset, end-1, u.Size)
	log.WithField("id", u.ID).Info("Uploading ", frags)
	request.Header.Add("Content-Range", frags)

	resp, err := client.Do(request)
	if err != nil {
		// this is a serious error, not simply one with a non-200 return code
		log.WithField(
			"id", u.ID,
		).Error("Error during file upload, terminating upload session.")
		return nil, -1, err
	}
	defer resp.Body.Close()
	response, _ := ioutil.ReadAll(resp.Body)
	return response, resp.StatusCode, nil
}

// Upload copies the file's contents to the server. Should only be called as a
// goroutine, or it can potentially block for a very long time.
func (u *UploadSession) Upload(auth *Auth) error {
	log.WithField("id", u.ID).Debug("Uploading file.")
	u.setState(started)
	if !u.isLargeSession() {
		resp, err := Put(
			fmt.Sprintf("/me/drive/items/%s/content", u.ID),
			auth,
			bytes.NewReader(u.data),
		)
		if err != nil && strings.Contains(err.Error(), "resourceModified") {
			// retry the request after a second, likely the server is having issues
			time.Sleep(time.Second)
			resp, err = Put(
				fmt.Sprintf("/me/drive/items/%s/content", u.ID),
				auth,
				bytes.NewReader(u.data),
			)
		}

		u.setState(complete)
		if err != nil {
			u.setState(errored)
			log.WithFields(log.Fields{
				"id":       u.ID,
				"response": string(resp),
				"err":      err,
			}).Error("Error during small file upload.")
		}
		return err
	}

	nchunks := int(math.Ceil(float64(u.Size) / float64(chunkSize)))
	for i := 0; i < nchunks; i++ {
		resp, status, err := u.uploadChunk(auth, uint64(i)*chunkSize)
		if err != nil {
			log.WithFields(log.Fields{
				"id":      u.ID,
				"chunk":   i,
				"nchunks": nchunks,
				"err":     err,
			}).Error("Error during chunk upload, cancelling upload session.")
			u.cancel(auth)
			return err
		}

		// retry server-side failures with an exponential back-off strategy
		for backoff := 1; status >= 500; backoff *= 2 {
			log.WithFields(log.Fields{
				"id":      u.ID,
				"chunk":   i,
				"nchunks": nchunks,
			}).Errorf("The OneDrive server is having issues, retrying upload in %ds.", backoff)
			resp, status, err = u.uploadChunk(auth, uint64(i)*chunkSize)
			if err != nil {
				log.WithFields(log.Fields{
					"id":       u.ID,
					"response": resp,
					"err":      err,
				}).Error("Failed while retrying upload. Killing upload session.")
				u.cancel(auth)
				return err
			}
		}

		// handle client-side errors
		if status == 404 {
			log.WithFields(log.Fields{
				"id":   u.ID,
				"code": status,
			}).Error("Upload session expired, cancelling upload.")
			// nothing to delete on the server, session expired
			u.setState(errored)
			return errors.New("Upload session expired")
		} else if status >= 400 {
			log.WithFields(log.Fields{
				"code":     status,
				"response": resp,
			}).Errorf(
				"Error code %d during upload. "+
					"Onedriver doesn't know how to handle this case yet. "+
					"Please file a bug report!",
				status,
			)
			u.setState(errored)
			return errors.New(string(resp))
		}
	}
	u.setState(complete)
	log.WithField("id", u.ID).Info("Upload completed!")
	return nil
}
