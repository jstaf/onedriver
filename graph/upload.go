package graph

// Although this could technically be part of drive_item.go, all the upload
// session stuff has been moved here to keep drive_item.go as readable as
// possible (due to the amount of upload-related code.)

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jstaf/onedriver/logger"
)

// 10MB is the recommended upload size according to the graph API docs
const chunkSize uint64 = 10 * 1024 * 1024

// UploadSession contains a snapshot of the file we're uploading. We have to
// take the snapshot or the file may have changed on disk during upload (which
// would break the upload).
type UploadSession struct {
	ID                 string    `json:"id"`
	UploadURL          string    `json:"uploadUrl"`
	ExpirationDateTime time.Time `json:"expirationDateTime"`
	data               *[]byte
	Size               uint64 `json:"-"`
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

// createUploadSession creates a new "upload session" resource on the server for
// uploading big files.
func (d *DriveItem) createUploadSession(auth Auth) (*UploadSession, error) {
	d.cancelUploadSession(auth) // THERE CAN ONLY BE ONE!

	sessionResp, _ := json.Marshal(UploadSessionPost{
		ConflictBehavior: "replace",
		FileSystemInfo: FileSystemInfo{
			LastModifiedDateTime: time.Unix(int64(d.ModTime()), 0),
		},
	})

	//TODO yikes, there has to be a way to upload by ID here... cmon microsoft.
	// (unless we can upload by id, an upload that gets mv-ed before it's
	// finished will do weird things locally)
	resp, err := Post(ResourcePath(d.Path())+":/createUploadSession",
		auth, bytes.NewReader(sessionResp))
	if err != nil {
		return nil, err
	}

	session := UploadSession{Size: d.Size()}
	err = json.Unmarshal(resp, &session)
	if err != nil {
		return nil, err
	}
	snapshot := make([]byte, session.Size)
	copy(snapshot, *d.data)
	session.data = &snapshot
	d.mutex.Lock()
	d.uploadSession = &session
	d.mutex.Unlock()
	return &session, nil
}

// cancel the upload session by deleting the temp file at the endpoint and
// clearing the singleton field in the DriveItem
func (d *DriveItem) cancelUploadSession(auth Auth) {
	d.mutex.Lock()
	if d.uploadSession != nil {
		// dont care about result, this is purely us being polite to the server
		go Delete(d.uploadSession.UploadURL, auth)
	}
	d.uploadSession = nil
	d.mutex.Unlock()
}

// Internal method used for uploading individual chunks of a DriveItem. We have
// to make things this way because the internal Put func doesn't work all that
// well when we need to add custom headers.
func (u UploadSession) uploadChunk(auth Auth, offset uint64) ([]byte, int, error) {
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
	request, _ := http.NewRequest("PUT",
		u.UploadURL, bytes.NewReader((*u.data)[offset:end]))
	// no Authorization header - it will throw a 401 if present
	request.Header.Add("Content-Length", strconv.Itoa(int(reqChunkSize)))
	frags := fmt.Sprintf("bytes %d-%d/%d", offset, end-1, u.Size)
	logger.Info("Uploading", frags)
	request.Header.Add("Content-Range", frags)

	resp, err := client.Do(request)
	if err != nil {
		// this is a serious error, not simply one with a non-200 return code
		logger.Error("Error during file upload, terminating upload session.")
		return nil, -1, err
	}
	defer resp.Body.Close()
	response, _ := ioutil.ReadAll(resp.Body)
	return response, resp.StatusCode, nil
}

// Upload copies the file's contents to the server. Should only be called as a
// goroutine, or it can potentially block for a very long time.
func (d *DriveItem) Upload(auth Auth) error {
	logger.Info(d.Path())

	size := d.Size()
	if size <= 4*1024*1024 { // 4MB
		// size is small enough that we can use a single PUT request

		// creating a snapshot prevents lock contention during the actual http
		// upload
		id, err := d.ID(auth)
		if err != nil || id == "" {
			logger.Error("Could not obtain ID for upload of", d.Name(), ", error:", err)
			return err
		}
		d.mutex.RLock()
		logger.Trace("Using simple upload for", d.Name())
		snapshot := make([]byte, size)
		copy(snapshot, *d.data)
		d.mutex.RUnlock()

		resp, err := Put("/me/drive/items/"+id+"/content", auth,
			bytes.NewReader(snapshot))

		d.mutex.Lock()
		defer d.mutex.Unlock()
		if err != nil {
			d.hasChanges = true
			return err
		}
		// Unmarshal into existing item so we don't have to redownload file contents.
		return json.Unmarshal(resp, d)
	}

	logger.Info("Creating upload session for", d.Name())
	session, err := d.createUploadSession(auth)
	if err != nil {
		logger.Error("Could not create upload session:", err)
		return err
	}

	nchunks := int(math.Ceil(float64(session.Size) / float64(chunkSize)))
	for i := 0; i < nchunks; i++ {
		resp, status, err := session.uploadChunk(auth, uint64(i)*chunkSize)
		if err != nil {
			logger.Errorf("Error while uploading chunk %d of %d: %s\n", i, nchunks, err)
			logger.Error("Cancelling upload session...")
			d.cancelUploadSession(auth)
			d.hasChanges = true
			return err
		}

		// retry server-side failures with an exponential back-off strategy
		for backoff := 1; status >= 500; backoff *= 2 {
			logger.Error("The OneDrive server is having issues, "+
				"retrying upload in %ds...", backoff)
			resp, status, err = session.uploadChunk(auth, uint64(i)*chunkSize)
			if err != nil {
				logger.Error(resp)
				logger.Error("Failed while retrying upload. Killing upload session...")
				d.cancelUploadSession(auth)
				d.hasChanges = true
				return err
			}
		}

		// handle client-side errors
		if status == 404 {
			logger.Error("Upload session expired, cancelling upload.")
			d.uploadSession = nil // nothing to delete on the server
			d.hasChanges = true
			return errors.New("Upload session expired")
		} else if status >= 400 {
			logger.Errorf("Error %d during upload: %s\n", status, resp)
			d.hasChanges = true
			return errors.New(string(resp))
		}
	}

	logger.Infof("Upload of %s completed!", d.Path())
	return nil
}
