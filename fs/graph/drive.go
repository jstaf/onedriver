package graph

import (
	"encoding/json"
	"net/url"
)

// DriveQuota is used to parse the User's current storage quotas from the API
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/quota
type DriveQuota struct {
	Deleted   uint64 `json:"deleted"`   // bytes in recycle bin
	FileCount uint64 `json:"fileCount"` // unavailable on personal accounts
	Remaining uint64 `json:"remaining"`
	State     string `json:"state"` // normal | nearing | critical | exceeded
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
}

// Drive has some general information about the user's OneDrive
// https://docs.microsoft.com/en-us/onedrive/developer/rest-api/resources/drive
type Drive struct {
	ID        string     `json:"id"`
	DriveType string     `json:"driveType"` // personal | business | documentLibrary
	Quota     DriveQuota `json:"quota,omitempty"`
}

type DriveList struct {
	Drives []*Drive `json:"value"`
}

// GetAllDrives fetches all drives a user has access to
func GetAllDrives(auth *Auth) ([]*Drive, error) {
	resp, err := Get("/me/drives", auth)
	drives := DriveList{}
	if err != nil {
		return drives.Drives, err
	}
	return drives.Drives, json.Unmarshal(resp, &drives)
}

// GetDrive is used to fetch the details of a specific drive
func GetDrive(id string, auth *Auth) (Drive, error) {
	endpoint := "/me/drive"
	if id != "me" {
		endpoint = "/drives/" + url.PathEscape(id)
	}
	resp, err := Get(endpoint, auth)
	drive := Drive{}
	if err != nil {
		return drive, err
	}
	return drive, json.Unmarshal(resp, &drive)
}
