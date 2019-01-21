package graph

/*
#cgo pkg-config: webkit2gtk-4.0
#include "stdlib.h"
#include "oauth2_gtk.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unsafe"
)

const authCodeURL = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
const authTokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
const authRedirectURL = "https://login.live.com/oauth20_desktop.srf"
const authClientID = "3470c3fa-bc10-45ab-a0a9-2d30836485d1"
const authFile = "auth_tokens.json"

// Auth represents a set of oauth2 authentication tokens
type Auth struct {
	ExpiresIn    int64  `json:"expires_in"` // only used for parsing
	ExpiresAt    int64  `json:"expires_at"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ToFile writes auth tokens to a file
func (a Auth) ToFile(file string) error {
	byteData, _ := json.Marshal(a)
	return ioutil.WriteFile(file, byteData, 0600)
}

// FromFile populates an auth struct from a file
func (a *Auth) FromFile(file string) error {
	contents, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, a)
}

// Refresh auth tokens if expired.
func (a *Auth) Refresh() {
	if a.ExpiresAt <= time.Now().Unix() {
		log.Println("Auth tokens expired, attempting renewal...")
		oldTime := a.ExpiresAt

		postData := strings.NewReader("client_id=" + authClientID +
			"&redirect_uri= " + authRedirectURL +
			"&refresh_token= " + a.RefreshToken +
			"&grant_type=refresh_token")
		resp, err := http.Post(authTokenURL,
			"application/x-www-form-urlencoded",
			postData)
		if err != nil {
			log.Fatal("Could not renew tokens, exiting...")
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)
		json.Unmarshal(body, &a)
		if a.ExpiresAt == oldTime {
			a.ExpiresAt = time.Now().Unix() + a.ExpiresIn
		}
		a.ToFile(authFile)
	}
}

// Fetch the auth code required as the first part of oauth2 authentication.
func getAuthCode() string {
	authURL := authCodeURL +
		"?client_id=" + authClientID +
		"&scope=" + "files.readwrite files.readwrite.all offline_access" +
		"&response_type=code" +
		"&redirect_uri=" + authRedirectURL

	cAuthURL := C.CString(authURL)
	defer C.free(unsafe.Pointer(cAuthURL))
	responseC := C.auth_window(cAuthURL)
	defer C.free(unsafe.Pointer(responseC))
	response := C.GoString(responseC)

	rexp := regexp.MustCompile("code=([a-zA-Z0-9-])+")
	code := rexp.FindString(response)
	if len(code) == 0 {
		log.Fatal("No validation code returned, or code was invalid. " +
			"Please restart the application and try again.")
	}
	return code[5:]
}

// Exchange an auth code for a set of access tokens
func getAuthTokens(authCode string) Auth {
	postData := strings.NewReader(
		"client_id=" + authClientID +
			"&redirect_uri=" + authRedirectURL +
			"&code=" + authCode +
			"&grant_type=authorization_code")
	resp, err := http.Post(authTokenURL,
		"application/x-www-form-urlencoded",
		postData)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var auth Auth
	json.Unmarshal(body, &auth)
	if auth.ExpiresAt == 0 {
		auth.ExpiresAt = time.Now().Unix() + auth.ExpiresIn
	}
	return auth
}

// Authenticate performs first-time authentication to Graph
func Authenticate() Auth {
	var auth Auth
	_, err := os.Stat(authFile)
	if os.IsNotExist(err) {
		// no tokens found, gotta start oauth flow from beginning
		code := getAuthCode()
		auth = getAuthTokens(code)
		auth.ToFile(authFile)
	} else {
		// we already have tokens, no need to force a refresh
		auth.FromFile(authFile)
		auth.Refresh()
	}
	return auth
}
