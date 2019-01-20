package onedriver

import (
	"testing"
	"time"
)

func TestAuthFromfile(t *testing.T) {
	var auth Auth
	auth.FromFile("auth_tokens.json")
	if auth.AccessToken == "" {
		t.Fatal("Could not load auth tokens from 'auth_tokens.json'! " +
			"Check that this file exists before running any more tests.")
	}
}

func TestAuthRefresh(t *testing.T) {
	var auth Auth
	auth.FromFile("auth_tokens.json")
	auth.ExpiresAt = 0 // force an auth refresh
	auth.Refresh()
	if auth.ExpiresAt <= time.Now().Unix() {
		t.Fatal("Auth could not be refreshed successfully!")
	}
}
