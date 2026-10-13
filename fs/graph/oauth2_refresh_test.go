package graph

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRefreshAuth(tokenURL string) *Auth {
	return &Auth{
		AuthConfig:   AuthConfig{ClientID: "id", RedirectURL: "http://x/?a=b&c=d", TokenURL: tokenURL},
		AccessToken:  "old-access",
		RefreshToken: "old+refresh&token=1",
		ExpiresAt:    0,
	}
}

func TestRefreshUpdatesInPlaceAndEncodesForm(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		form = r.PostForm
		w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	defer srv.Close()

	auth := testRefreshAuth(srv.URL)
	auth.path = filepath.Join(t.TempDir(), "auth.json")
	same := auth // another holder of the same *Auth
	auth.Refresh()

	assert.Equal(t, "old+refresh&token=1", form.Get("refresh_token"))
	assert.Equal(t, "http://x/?a=b&c=d", form.Get("redirect_uri"))
	assert.Equal(t, "refresh_token", form.Get("grant_type"))
	assert.Equal(t, "new-access", same.AccessToken)
	assert.Equal(t, "new-refresh", same.RefreshToken)
	assert.Greater(t, same.ExpiresAt, time.Now().Unix())
	assert.FileExists(t, auth.path)
}

func TestRefreshTransientErrorKeepsTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	auth := testRefreshAuth(srv.URL)

	assert.False(t, auth.refresh(""), "a 503 is not a rejected login")
	assert.Equal(t, "old-access", auth.AccessToken)
	assert.Equal(t, "old+refresh&token=1", auth.RefreshToken)

	// the network going away entirely is transient too
	srv.Close()
	auth.nextRetry = time.Time{}
	assert.False(t, auth.refresh(""))
	assert.Equal(t, "old-access", auth.AccessToken)
}

func TestRefreshRejectedByServer(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant","error_description":"expired"}`))
	}))
	defer srv.Close()
	auth := testRefreshAuth(srv.URL)

	require.True(t, auth.refresh(""))
	assert.Equal(t, "old-access", auth.AccessToken, "tokens must be left alone")
	assert.Equal(t, 1, calls)

	// no hammering of the token endpoint right after a failure
	assert.False(t, auth.refresh(""))
	assert.Equal(t, 1, calls)
}
