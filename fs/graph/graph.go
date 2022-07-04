// Package graph provides the basic APIs to interact with Microsoft Graph. This includes
// the DriveItem resource and supporting resources which are the basis of working with
// files and folders through the Microsoft Graph API.
package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/imdario/mergo"
	"github.com/rs/zerolog/log"
)

// GraphURL is the API endpoint of Microsoft Graph
const GraphURL = "https://graph.microsoft.com/v1.0"

// graphError is an internal struct used when decoding Graph's error messages
type graphError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Request performs an authenticated request to Microsoft Graph
func Request(resource string, auth *Auth, method string, content io.Reader) ([]byte, error) {
	if auth == nil || auth.AccessToken == "" {
		// a catch all condition to avoid wiping our auth by accident
		log.Error().Msg("Auth was empty and we attempted to make a request with it!")
		return nil, errors.New("cannot make a request with empty auth")
	}

	auth.Refresh()

	client := &http.Client{Timeout: 15 * time.Second}
	request, _ := http.NewRequest(method, GraphURL+resource, content)
	request.Header.Add("Authorization", "bearer "+auth.AccessToken)
	switch method { // request type-specific code here
	case "PATCH":
		request.Header.Add("If-Match", "*")
		request.Header.Add("Content-Type", "application/json")
	case "POST":
		request.Header.Add("Content-Type", "application/json")
	case "PUT":
		request.Header.Add("Content-Type", "text/plain")
	}

	response, err := client.Do(request)
	if err != nil {
		// the actual request failed
		return nil, err
	}
	body, _ := ioutil.ReadAll(response.Body)
	response.Body.Close()

	if response.StatusCode == 401 {
		var err graphError
		json.Unmarshal(body, &err)
		log.Warn().
			Str("code", err.Error.Code).
			Str("message", err.Error.Message).
			Msg("Authentication token invalid or new app permissions required, " +
				"forcing reauth before retrying.")

		reauth := newAuth(auth.AuthConfig, auth.path, false)
		mergo.Merge(auth, reauth, mergo.WithOverride)
		request.Header.Set("Authorization", "bearer "+auth.AccessToken)
	}
	if response.StatusCode >= 500 || response.StatusCode == 401 {
		// the onedrive API is having issues, retry once
		response, err = client.Do(request)
		if err != nil {
			return nil, err
		}
		body, _ = ioutil.ReadAll(response.Body)
		response.Body.Close()
	}

	if response.StatusCode >= 400 {
		// something was wrong with the request
		var err graphError
		json.Unmarshal(body, &err)
		return nil, fmt.Errorf("HTTP %d - %s: %s",
			response.StatusCode, err.Error.Code, err.Error.Message)
	}
	return body, nil
}

// Get is a convenience wrapper around Request
func Get(resource string, auth *Auth) ([]byte, error) {
	return Request(resource, auth, "GET", nil)
}

// Patch is a convenience wrapper around Request
func Patch(resource string, auth *Auth, content io.Reader) ([]byte, error) {
	return Request(resource, auth, "PATCH", content)
}

// Post is a convenience wrapper around Request
func Post(resource string, auth *Auth, content io.Reader) ([]byte, error) {
	return Request(resource, auth, "POST", content)
}

// Put is a convenience wrapper around Request
func Put(resource string, auth *Auth, content io.Reader) ([]byte, error) {
	return Request(resource, auth, "PUT", content)
}

// Delete performs an HTTP delete
func Delete(resource string, auth *Auth) error {
	_, err := Request(resource, auth, "DELETE", nil)
	return err
}

// IDPath computes the resource path for an item by ID
func IDPath(id string) string {
	if id == "root" {
		return "/me/drive/root"
	}
	return "/me/drive/items/" + url.PathEscape(id)
}

// ResourcePath translates an item's path to the proper path used by Graph
func ResourcePath(path string) string {
	if path == "/" {
		return "/me/drive/root"
	}
	return "/me/drive/root:" + url.PathEscape(path)
}

// ChildrenPath returns the path to an item's children
func childrenPath(path string) string {
	if path == "/" {
		return ResourcePath(path) + "/children"
	}
	return ResourcePath(path) + ":/children"
}

// ChildrenPathID returns the API resource path of an item's children
func childrenPathID(id string) string {
	return fmt.Sprintf("/me/drive/items/%s/children", url.PathEscape(id))
}

// User represents the user. Currently only used to fetch the account email so
// we can display it in file managers with .xdg-volume-info
// https://docs.microsoft.com/en-ca/graph/api/user-get
type User struct {
	UserPrincipalName string `json:"userPrincipalName"`
}

// GetUser fetches the current user details from the Graph API.
func GetUser(auth *Auth) (User, error) {
	resp, err := Get("/me", auth)
	user := User{}
	if err == nil {
		err = json.Unmarshal(resp, &user)
	}
	return user, err
}

// IsOffline checks if an error string from Request() is indicative of being offline.
func IsOffline(err error) bool {
	if err == nil {
		return false
	}
	// our error messages from Request() will be prefixed with "HTTP ### -" if we actually
	// got an HTTP response (indicating we are not offline)
	rexp := regexp.MustCompile("HTTP [0-9]+ - ")
	return !rexp.MatchString(err.Error())
}
