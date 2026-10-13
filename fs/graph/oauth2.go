package graph

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"dario.cat/mergo"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// these are default values if not specified
const (
	authClientID    = "3470c3fa-bc10-45ab-a0a9-2d30836485d1"
	authCodeURL     = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	authTokenURL    = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	authRedirectURL = "https://login.live.com/oauth20_desktop.srf"
)

func (a *AuthConfig) applyDefaults() error {
	return mergo.Merge(a, AuthConfig{
		ClientID:    authClientID,
		CodeURL:     authCodeURL,
		TokenURL:    authTokenURL,
		RedirectURL: authRedirectURL,
	})
}

// AuthConfig configures the authentication flow
type AuthConfig struct {
	ClientID    string `json:"clientID" yaml:"clientID"`
	CodeURL     string `json:"codeURL" yaml:"codeURL"`
	TokenURL    string `json:"tokenURL" yaml:"tokenURL"`
	RedirectURL string `json:"redirectURL" yaml:"redirectURL"`
}

// Auth represents a set of oauth2 authentication tokens
type Auth struct {
	AuthConfig   `json:"config"`
	Account      string    `json:"account"`
	ExpiresIn    int64     `json:"expires_in"` // only used for parsing
	ExpiresAt    int64     `json:"expires_at"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	path         string    // auth tokens remember their path for use by Refresh()
	nextRetry    time.Time // don't try to renew tokens again before this (see refresh)
}

// AuthError is an authentication error from the Microsoft API. Generally we don't see
// these unless something goes catastrophically wrong with Microsoft's authentication
// services.
type AuthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorCodes       []int  `json:"error_codes"`
	ErrorURI         string `json:"error_uri"`
	Timestamp        string `json:"timestamp"` // json.Unmarshal doesn't like this timestamp format
	TraceID          string `json:"trace_id"`
	CorrelationID    string `json:"correlation_id"`
}

// ToFile writes auth tokens to a file
func (a Auth) ToFile(file string) error {
	a.path = file
	byteData, _ := json.Marshal(a)
	return os.WriteFile(file, byteData, 0600)
}

// FromFile populates an auth struct from a file
func (a *Auth) FromFile(file string) error {
	contents, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	a.path = file
	err = json.Unmarshal(contents, a)
	if err != nil {
		return err
	}
	return a.applyDefaults()
}

// serializes token renewals so concurrent requests don't all renew at once
var refreshMu sync.Mutex

// Refresh auth tokens if expired. This is safe to call from the background:
// it never starts an interactive login. If the network is down or the server
// is having problems, the current tokens are kept and renewal is tried again
// later. If the server definitively rejects the refresh token, an error is
// logged and the tokens are left alone (the user has to run
// "onedriver --auth-only" to log in again); the filesystem keeps running from
// its cache in the meantime.
func (a *Auth) Refresh() {
	a.refresh("")
}

// how long to wait before trying to renew tokens again after an error
const refreshRetryDelay = 10 * time.Second

// refresh renews the tokens in place. If usedToken is empty, this only happens
// if the access token is past its expiry time. Otherwise the server rejected
// usedToken, and tokens are renewed regardless of their expiry time unless
// someone else did that already. It returns true if the server definitively
// rejected our refresh token, which only a new login can fix.
func (a *Auth) refresh(usedToken string) (rejected bool) {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	if usedToken == "" && a.ExpiresAt > time.Now().Unix() {
		return false
	}
	if usedToken != "" && a.AccessToken != usedToken {
		return false // already renewed by someone else
	}
	if time.Now().Before(a.nextRetry) {
		return false
	}

	tokens, transient, err := postTokenRequest(a.TokenURL, url.Values{
		"client_id":     {a.ClientID},
		"redirect_uri":  {a.RedirectURL},
		"refresh_token": {a.RefreshToken},
		"grant_type":    {"refresh_token"},
	})
	if err != nil {
		a.nextRetry = time.Now().Add(refreshRetryDelay)
		if transient {
			if IsOffline(err) {
				// don't spam the log while offline
				log.Trace().Err(err).Msg("Network unreachable during token renewal, ignoring.")
			} else {
				log.Warn().Err(err).Msg("Could not renew auth tokens, will retry later.")
			}
			return false
		}
		log.Error().Err(err).Msg("The server rejected our auth tokens. " +
			"Run \"onedriver --auth-only\" to log in again.")
		return true
	}

	// update in place, other holders of this *Auth need to see the new tokens
	a.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		a.RefreshToken = tokens.RefreshToken
	}
	a.ExpiresIn = tokens.ExpiresIn
	a.ExpiresAt = time.Now().Unix() + tokens.ExpiresIn
	a.nextRetry = time.Time{}
	if a.path != "" {
		if err := a.ToFile(a.path); err != nil {
			log.Error().Err(err).Msg("Could not save renewed auth tokens.")
		}
	}
	return false
}

// postTokenRequest posts a form to the token endpoint. On failure, transient
// says if trying again later might help (network problems, server errors) as
// opposed to the server definitively refusing the request (invalid_grant etc).
func postTokenRequest(tokenURL string, form url.Values) (tokens *Auth, transient bool, err error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(tokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var parsed Auth
	json.Unmarshal(body, &parsed)
	if resp.StatusCode == http.StatusOK && parsed.AccessToken != "" {
		return &parsed, false, nil
	}

	var authErr AuthError
	json.Unmarshal(body, &authErr)
	err = fmt.Errorf("token endpoint returned HTTP %d: %s %s",
		resp.StatusCode, authErr.Error, authErr.ErrorDescription)
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		if authErr.Error == "temporarily_unavailable" || authErr.Error == "server_error" {
			return nil, true, err
		}
		return nil, false, err
	default:
		// 5xx, 429, ...
		return nil, true, err
	}
}

// Get the appropriate authentication URL for the Graph OAuth2 challenge.
func getAuthURL(a AuthConfig) string {
	return a.CodeURL +
		"?client_id=" + a.ClientID +
		"&scope=" + url.PathEscape("user.read files.readwrite.all offline_access") +
		"&response_type=code" +
		"&redirect_uri=" + a.RedirectURL
}

// getAuthCodeHeadless has the user perform authentication in their own browser
// instead of WebKit2GTK and then input the auth code in the terminal.
func getAuthCodeHeadless(a AuthConfig, accountName string) string {
	fmt.Printf("Please visit the following URL:\n%s\n\n", getAuthURL(a))
	fmt.Println("Please enter the redirect URL once you are redirected to a " +
		"blank page (after \"Let this app access your info?\"):")
	var response string
	fmt.Scanln(&response)
	code, err := parseAuthCode(response)
	if err != nil {
		log.Fatal().Msg("No validation code returned, or code was invalid. " +
			"Please restart the application and try again.")
	}
	return code
}

// parseAuthCode is used to parse the auth code out of the redirect the server gives us
// after successful authentication
func parseAuthCode(authURL string) (string, error) {
	parsed, err := url.Parse(authURL)
	if err != nil {
		return "", err
	}
	params := parsed.Query()
	return params.Get("code"), nil
}

// Exchange an auth code for a set of access tokens (returned as a new Auth struct).
func getAuthTokens(a AuthConfig, authCode string) *Auth {
	postData := strings.NewReader(url.Values{
		"client_id":    {a.ClientID},
		"redirect_uri": {a.RedirectURL},
		"code":         {authCode},
		"grant_type":   {"authorization_code"},
	}.Encode())
	resp, err := http.Post(a.TokenURL,
		"application/x-www-form-urlencoded",
		postData)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not POST to obtain auth tokens.")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var auth Auth
	json.Unmarshal(body, &auth)
	if auth.ExpiresAt == 0 {
		auth.ExpiresAt = time.Now().Unix() + auth.ExpiresIn
	}
	auth.AuthConfig = a

	if auth.AccessToken == "" || auth.RefreshToken == "" {
		var authErr AuthError
		var fields zerolog.Logger
		if err := json.Unmarshal(body, &authErr); err == nil {
			// we got a parseable error message out of microsoft's servers
			fields = log.With().
				Int("status", resp.StatusCode).
				Str("error", authErr.Error).
				Str("errorDescription", authErr.ErrorDescription).
				Str("helpUrl", authErr.ErrorURI).
				Logger()
		} else {
			// things are extra broken and this is an error type we haven't seen before
			fields = log.With().
				Int("status", resp.StatusCode).
				Bytes("response", body).
				Err(err).
				Logger()
		}
		fields.Fatal().Msg(
			"Failed to retrieve access tokens. Authentication cannot continue.",
		)
	}
	return &auth
}

// newAuth performs initial authentication flow and saves tokens to disk. The headless
// parameter determines if we will try to auth directly in the terminal instead of
// doing it via embedded browser.
func newAuth(config AuthConfig, path string, headless bool) *Auth {
	// load the old account name
	old := Auth{}
	old.FromFile(path)

	config.applyDefaults()
	var code string
	if headless {
		code = getAuthCodeHeadless(config, old.Account)
	} else {
		// in a build without CGO, this will be the same as above
		code = getAuthCode(config, old.Account)
	}
	auth := getAuthTokens(config, code)

	if user, err := GetUser(auth); err == nil {
		auth.Account = user.UserPrincipalName
	}
	auth.ToFile(path)
	return auth
}

// Authenticate performs authentication to Graph or load auth/refreshes it
// from an existing file. If headless is true, we will authenticate in the
// terminal.
func Authenticate(config AuthConfig, path string, headless bool) *Auth {
	auth := &Auth{}
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// no tokens found, gotta start oauth flow from beginning
		auth = newAuth(config, path, headless)
	} else {
		// we already have tokens, no need to force a new auth flow
		auth.FromFile(path)
		if auth.refresh("") {
			// we are in the foreground at startup, so it is fine to ask the
			// user to log in again here (unlike in Refresh())
			log.Warn().Msg("Failed to renew access tokens. Attempting to reauthenticate.")
			auth = newAuth(auth.AuthConfig, path, headless)
		}
	}
	return auth
}
