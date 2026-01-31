package fs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog/log"
)

const (
	// 10MB is the recommended upload size according to the graph API docs
	uploadChunkSize uint64 = 10 * 1024 * 1024

	// uploads larget than 4MB must use a formal upload session
	uploadLargeSize uint64 = 4 * 1024 * 1024
)

// upload states
const (
	uploadNotStarted = iota
	uploadStarted
	uploadComplete
	uploadErrored
)

// UploadSession contains a snapshot of the file we're uploading. We have to
// take the snapshot or the file may have changed on disk during upload (which
// would break the upload). It is not recommended to directly deserialize into
// this structure from API responses in case Microsoft ever adds a size, data,
// or modTime field to the response.
type UploadSession struct {
	ID                 string    `json:"id"`
	OldID              string    `json:"oldID"`
	ParentID           string    `json:"parentID"`
	NodeID             uint64    `json:"nodeID"`
	Name               string    `json:"name"`
	ExpirationDateTime time.Time `json:"expirationDateTime"`
	Size               uint64    `json:"size,omitempty"`
	Data               []byte    `json:"data,omitempty"`
	QuickXORHash       string    `json:"quickxorhash,omitempty"`
	ModTime            time.Time `json:"modTime,omitempty"`
	retries            int

	sync.Mutex
	UploadURL string `json:"uploadUrl"`
	ETag      string `json:"eTag,omitempty"`
	state     int
	error     // embedded error tracks errors that killed an upload
}

// MarshalJSON implements a custom JSON marshaler to avoid race conditions
func (u *UploadSession) MarshalJSON() ([]byte, error) {
	u.Lock()
	defer u.Unlock()
	type SerializeableUploadSession UploadSession
	return json.Marshal((*SerializeableUploadSession)(u))
}

// UploadSessionPost is the initial post used to create an upload session
type UploadSessionPost struct {
	Name             string `json:"name,omitempty"`
	ConflictBehavior string `json:"@microsoft.graph.conflictBehavior,omitempty"`
	FileSystemInfo   `json:"fileSystemInfo,omitempty"`
}

// FileSystemInfo carries the filesystem metadata like Mtime/Atime
type FileSystemInfo struct {
	LastModifiedDateTime time.Time `json:"lastModifiedDateTime,omitempty"`
}

func (u *UploadSession) getState() int {
	u.Lock()
	defer u.Unlock()
	return u.state
}

// setState is just a helper method to set the UploadSession state and make error checking
// a little more straightforwards.
func (u *UploadSession) setState(state int, err error) error {
	u.Lock()
	u.state = state
	u.error = err
	u.Unlock()
	return err
}

// NewUploadSession wraps an upload of a file into an UploadSession struct
// responsible for performing uploads for a file.
func NewUploadSession(inode *Inode, data *[]byte) (*UploadSession, error) {
	if data == nil {
		return nil, errors.New("data to upload cannot be nil")
	}

	// create a generic session for all files
	inode.RLock()
	session := UploadSession{
		ID:       inode.DriveItem.ID,
		OldID:    inode.DriveItem.ID,
		ParentID: inode.DriveItem.Parent.ID,
		NodeID:   inode.nodeID,
		Name:     inode.DriveItem.Name,
		Data:     *data,
		ModTime:  *inode.DriveItem.ModTime,
	}
	inode.RUnlock()

	session.Size = uint64(len(*data)) // just in case it somehow differs
	session.QuickXORHash = graph.QuickXORHash(data)
	return &session, nil
}

// cancel the upload session by deleting the temp file at the endpoint.
func (u *UploadSession) cancel(auth *graph.Auth) {
	u.Lock()
	// small upload sessions will also have an empty UploadURL in addition to
	// uninitialized large file uploads.
	nonemptyURL := u.UploadURL != ""
	u.Unlock()
	if nonemptyURL {
		state := u.getState()
		if state == uploadStarted || state == uploadErrored {
			// dont care about result, this is purely us being polite to the server
			go graph.Delete(u.UploadURL, auth)
		}
	}
}

// Internal method used for uploading individual chunks of a DriveItem. We have
// to make things this way because the internal Put func doesn't work all that
// well when we need to add custom headers. Will return without an error if
// irrespective of HTTP status (errors are reserved for stuff that prevented
// the HTTP request at all).
func (u *UploadSession) uploadChunk(auth *graph.Auth, offset uint64) ([]byte, int, error) {
	u.Lock()
	url := u.UploadURL
	if url == "" {
		u.Unlock()
		return nil, -1, errors.New("UploadSession UploadURL cannot be empty")
	}
	u.Unlock()

	// how much of the file are we going to upload?
	end := offset + uploadChunkSize
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
		url,
		bytes.NewReader((u.Data)[offset:end]),
	)
	// no Authorization header - it will throw a 401 if present
	request.Header.Add("Content-Length", strconv.Itoa(int(reqChunkSize)))
	frags := fmt.Sprintf("bytes %d-%d/%d", offset, end-1, u.Size)
	log.Info().Str("id", u.ID).Msg("Uploading " + frags)
	request.Header.Add("Content-Range", frags)

	resp, err := client.Do(request)
	if err != nil {
		// this is a serious error, not simply one with a non-200 return code
		return nil, -1, err
	}
	defer resp.Body.Close()
	response, _ := io.ReadAll(resp.Body)
	return response, resp.StatusCode, nil
}

// Upload copies the file's contents to the server. Should only be called as a
// goroutine, or it can potentially block for a very long time. The uploadSession.error
// field contains errors to be handled if called as a goroutine.
func (u *UploadSession) Upload(auth *graph.Auth) error {
	log.Info().Str("id", u.ID).Str("name", u.Name).Msg("Uploading file.")
	u.setState(uploadStarted, nil)

	var uploadPath string
	var resp []byte
	if u.Size < uploadLargeSize {
		// Small upload sessions use a simple PUT request, but this does not support
		// adding file modification times. We don't really care though, because
		// after some experimentation, the Microsoft API doesn't seem to properly
		// support these either (this is why we have to use etags).
		if isLocalID(u.ID) {
			uploadPath = fmt.Sprintf(
				"/me/drive/items/%s:/%s:/content",
				url.PathEscape(u.ParentID),
				url.PathEscape(u.Name),
			)
		} else {
			uploadPath = fmt.Sprintf(
				"/me/drive/items/%s/content",
				url.PathEscape(u.ID),
			)
		}
		// small files handled in this block
		var err error
		resp, err = graph.Put(uploadPath, auth, bytes.NewReader(u.Data))
		if err != nil && strings.Contains(err.Error(), "resourceModified") {
			// retry the request after a second, likely the server is having issues
			time.Sleep(time.Second)
			resp, err = graph.Put(uploadPath, auth, bytes.NewReader(u.Data))
		}
		if err != nil {
			return u.setState(uploadErrored, fmt.Errorf("small upload failed: %w", err))
		}
	} else {
		if isLocalID(u.ID) {
			uploadPath = fmt.Sprintf(
				"/me/drive/items/%s:/%s:/createUploadSession",
				url.PathEscape(u.ParentID),
				url.PathEscape(u.Name),
			)
		} else {
			uploadPath = fmt.Sprintf(
				"/me/drive/items/%s/createUploadSession",
				url.PathEscape(u.ID),
			)
		}
		sessionPostData, _ := json.Marshal(UploadSessionPost{
			ConflictBehavior: "replace",
			FileSystemInfo: FileSystemInfo{
				LastModifiedDateTime: u.ModTime,
			},
		})
		resp, err := graph.Post(uploadPath, auth, bytes.NewReader(sessionPostData))
		if err != nil {
			return u.setState(uploadErrored, fmt.Errorf("failed to create upload session: %w", err))
		}

		// populate UploadURL/expiration - we unmarshal into a fresh session here
		// just in case the API does something silly at a later date and overwrites
		// a field it shouldn't.
		tmp := UploadSession{}
		if err = json.Unmarshal(resp, &tmp); err != nil {
			return u.setState(uploadErrored,
				fmt.Errorf("could not unmarshal upload session post response: %w", err))
		}
		u.Lock()
		u.UploadURL = tmp.UploadURL
		u.ExpirationDateTime = tmp.ExpirationDateTime
		u.Unlock()

		// api upload session created successfully, now do actual content upload
		var status int
		nchunks := int(math.Ceil(float64(u.Size) / float64(uploadChunkSize)))
		for i := 0; i < nchunks; i++ {
			resp, status, err = u.uploadChunk(auth, uint64(i)*uploadChunkSize)
			if err != nil {
				return u.setState(uploadErrored, fmt.Errorf("failed to perform chunk upload: %w", err))
			}

			// retry server-side failures with an exponential back-off strategy. Will not
			// exit this loop unless it receives a non 5xx error or serious failure
			for backoff := 1; status >= 500; backoff *= 2 {
				log.Error().
					Str("id", u.ID).
					Str("name", u.Name).
					Int("chunk", i).
					Int("nchunks", nchunks).
					Int("status", status).
					Msgf("The OneDrive server is having issues, retrying chunk upload in %ds.", backoff)
				time.Sleep(time.Duration(backoff) * time.Second)
				resp, status, err = u.uploadChunk(auth, uint64(i)*uploadChunkSize)
				if err != nil { // a serious, non 4xx/5xx error
					return u.setState(uploadErrored, fmt.Errorf("failed to perform chunk upload: %w", err))
				}
			}

			// handle client-side errors
			if status >= 400 {
				return u.setState(uploadErrored, fmt.Errorf("error uploading chunk - HTTP %d: %s", status, string(resp)))
			}
		}
	}

	// server has indicated that the upload was successful - now we check to verify the
	// checksum is what it's supposed to be.
	remote := graph.DriveItem{}
	if err := json.Unmarshal(resp, &remote); err != nil {
		if len(resp) == 0 {
			// the API frequently just returns a 0-byte response for completed
			// multipart uploads, so we manually fetch the newly updated item
			var remotePtr *graph.DriveItem
			if isLocalID(u.ID) {
				remotePtr, err = graph.GetItemChild(u.ParentID, u.Name, auth)
			} else {
				remotePtr, err = graph.GetItem(u.ID, auth)
			}
			if err == nil {
				remote = *remotePtr
			} else {
				return u.setState(uploadErrored,
					fmt.Errorf("failed to get item post-upload: %w", err))
			}
		} else {
			return u.setState(uploadErrored,
				fmt.Errorf("could not unmarshal response: %w: %s", err, string(resp)),
			)
		}
	}
	if remote.File == nil && remote.Size != u.Size {
		// if we are absolutely pounding the microsoft API, a remote item may sometimes
		// come back without checksums, so we check the size of the uploaded item instead.
		return u.setState(uploadErrored, errors.New("size mismatch when remote checksums did not exist"))
	} else if !remote.VerifyChecksum(u.QuickXORHash) {
		return u.setState(uploadErrored, errors.New("remote checksum did not match"))
	}
	// update the UploadSession's ID in the event that we exchange a local for a remote ID
	u.Lock()
	u.ID = remote.ID
	u.ETag = remote.ETag
	u.Unlock()
	return u.setState(uploadComplete, nil)
}
