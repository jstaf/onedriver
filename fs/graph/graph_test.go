package graph

import (
	"testing"
	"time"
)

func TestRequestUnauthenticated(t *testing.T) {
	t.Parallel()
	badAuth := &Auth{
		// Set a renewal 1 year in the future so we don't accidentally overwrite
		// our auth tokens
		ExpiresAt: time.Now().Unix() + 60*60*24*365,
	}
	_, err := Get("/me/drive/root", badAuth)
	if err == nil {
		t.Fatal("An unauthenticated request was not handled as an error")
	}
}
