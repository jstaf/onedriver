package graph

// Although this could technically be part of drive_item.go, all the upload
// session stuff has been moved here to keep drive_item.go as readable as
// possible (due to the amount of upload-related code.)

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/jstaf/onedriver/logger"
)

// 10MB is the recommended upload size according to the graph API docs
const chunkSize uint64 = 10 * 1024 * 1024

// FileSystemInfo carries the filesystem metadata like Mtime/Atime
type FileSystemInfo struct {
	CreatedDateTime      time.Time `json:"createdDateTime,omitempty"`
	LastAccessedDateTime time.Time `json:"lastAccessedDateTime,omitempty"`
	LastModifiedDateTime time.Time `json:"lastModifiedDateTime,omitempty"`
}

// UploadSession is the initial post used to create an upload session
type UploadSession struct {
	Name             string `json:"name,omitempty"`
	ConflictBehavior string `json:"@microsoft.graph.conflictBehavior,omitempty"`
	FileSystemInfo   `json:"fileSystemInfo,omitempty"`
}

// UploadSessionResponse is created on a successful POST
type UploadSessionResponse struct {
	UploadURL          string    `json:"uploadUrl"`
	ExpirationDateTime time.Time `json:"expirationDateTime"`
}

// createUploadSession creates a new "upload session" resource on the server for
// uploading big files.
func (d *DriveItem) createUploadSession(auth Auth) error {
	d.cancelUploadSession(auth) // THERE CAN ONLY BE ONE!

	if d.ID == "" {
		return errors.New("id cannot be empty")
	}

	session, _ := json.Marshal(UploadSession{
		ConflictBehavior: "replace",
		FileSystemInfo: FileSystemInfo{
			LastModifiedDateTime: *d.ModifyTime,
		},
	})
	resp, err := Post("/me/drive/items/"+d.ID+"/createUploadSession",
		auth, bytes.NewReader(session))
	if err != nil {
		return err
	}

	dest := UploadSessionResponse{}
	err = json.Unmarshal(resp, &dest)
	if err != nil {
		return err
	}
	d.uploadSessionURL = dest.UploadURL
	return nil
}

// cancel the upload session by deleting the temp file at the endpoint and
// clearing the singleton field in the DriveItem
func (d *DriveItem) cancelUploadSession(auth Auth) {
	if d.uploadSessionURL != "" {
		Delete(d.uploadSessionURL, auth)
	}
	d.uploadSessionURL = ""
}

// Internal method used for uploading individual chunks of a DriveItem. We have
// to make things this way because the internal Put func doesn't work all that
// well when we need to add custom headers.
func (d DriveItem) uploadChunk(auth Auth, offset uint64) (int, error) {
	if d.uploadSessionURL == "" {
		return -1, errors.New("uploadSessionURL cannot be empty")
	}

	// how much of the file are we going to upload?
	end := offset + chunkSize
	var reqChunkSize uint64
	if end > d.Size {
		end = d.Size + 1
		reqChunkSize = end - offset
	}
	if offset > d.Size {
		return -1, errors.New("offset cannot be larger than DriveItem size")
	}

	auth.Refresh()

	client := &http.Client{}
	request, _ := http.NewRequest("PUT",
		d.uploadSessionURL, bytes.NewReader((*d.data)[offset:end]))
	// no Authorization header - it will throw a 401 if present
	request.Header.Add("Content-Length", string(reqChunkSize))
	request.Header.Add("Content-Range",
		fmt.Sprintf("bytes %d-%d/%d", offset, end, d.Size))

	resp, err := client.Do(request)
	if err != nil {
		// this is a serious error, not simply one with a non-200 return code
		logger.Error("Error during file upload, terminating upload session.")
		return -1, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// Upload copies the file's contents to the server. Should only be called as a
// goroutine, or it can potentially block for a very long time.
func (d *DriveItem) Upload(auth Auth) error {
	logger.Info(d.Name)
	d.ensureID(auth)
	if d.Size <= 4*1024*1024 { // 4MB
		// size is small enough that we can use a single PUT request
		logger.Trace("Using simple upload for", d.Name)
		resp, err := Put("/me/drive/items/"+d.ID+"/content", auth,
			bytes.NewReader(*d.data))
		if err != nil {
			return err
		}
		// Unmarshal into existing item so we don't have to redownload file contents.
		return json.Unmarshal(resp, d)
	}

	logger.Info("Creating upload session for", d.Name)
	if err := d.createUploadSession(auth); err != nil {
		logger.Error("Could not create upload session:", err)
		return err
	}

	nchunks := int(math.Ceil(float64(d.Size) / float64(chunkSize)))
	for i := 0; i < nchunks; i++ {
		status, err := d.uploadChunk(auth, uint64(i)*chunkSize)
		if err != nil {
			logger.Errorf("Error while uploading chunk %d of %d: %s\n", i, nchunks, err)
			logger.Error("Cancelling upload session...")
			d.cancelUploadSession(auth)
			return err
		}

		// retry server-side failures with an exponential back-off strategy
		for backoff := 1; status >= 500; backoff *= 2 {
			logger.Error("The OneDrive server is having issues, "+
				"retrying upload in %ds...", backoff)
			status, err = d.uploadChunk(auth, uint64(i)*chunkSize)
			if err != nil {
				logger.Error("Failed while retrying upload. Killing upload session...")
				d.cancelUploadSession(auth)
				return err
			}
		}

		if status == 404 {
			logger.Error("Upload session expired, cancelling upload.")
			d.uploadSessionURL = "" // nothing to delete on the server
			return errors.New("Upload session expired")
		}
	}

	logger.Infof("Upload of %s completed!", d.Path())
	return nil
}
