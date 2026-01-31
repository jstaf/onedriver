package graph

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
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
	Account      string `json:"account"`
	ExpiresIn    int64  `json:"expires_in"` // only used for parsing
	ExpiresAt    int64  `json:"expires_at"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	path         string // auth tokens remember their path for use by Refresh()
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
	return ioutil.WriteFile(file, byteData, 0600)
}

// FromFile populates an auth struct from a file
func (a *Auth) FromFile(file string) error {
	contents, err := ioutil.ReadFile(file)
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

// Refresh auth tokens if expired.
func (a *Auth) Refresh() {
	if a.ExpiresAt <= time.Now().Unix() {
		oldTime := a.ExpiresAt
		postData := strings.NewReader("client_id=" + a.ClientID +
			"&redirect_uri=" + a.RedirectURL +
			"&refresh_token=" + a.RefreshToken +
			"&grant_type=refresh_token")
		resp, err := http.Post(a.TokenURL,
			"application/x-www-form-urlencoded",
			postData)

		var reauth bool
		if err != nil {
			if IsOffline(err) || resp == nil {
				log.Trace().Err(err).Msg("Network unreachable during token renewal, ignoring.")
				return
			}
			log.Error().Err(err).Msg("Could not POST to renew tokens, forcing reauth.")
			reauth = true
		} else {
			// put here so as to avoid spamming the log when offline
			log.Info().Msg("Auth tokens expired, attempting renewal.")
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)
		json.Unmarshal(body, &a)
		if a.ExpiresAt == oldTime {
			a.ExpiresAt = time.Now().Unix() + a.ExpiresIn
		}

		if reauth || a.AccessToken == "" || a.RefreshToken == "" {
			log.Error().
				Bytes("response", body).
				Int("http_code", resp.StatusCode).
				Msg("Failed to renew access tokens. Attempting to reauthenticate.")
			a = newAuth(a.AuthConfig, a.path, false)
		} else {
			a.ToFile(a.path)
		}
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
	postData := strings.NewReader("client_id=" + a.ClientID +
		"&redirect_uri=" + a.RedirectURL +
		"&code=" + authCode +
		"&grant_type=authorization_code")
	resp, err := http.Post(a.TokenURL,
		"application/x-www-form-urlencoded",
		postData)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not POST to obtain auth tokens.")
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
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
		auth.Refresh()
	}
	return auth
}
