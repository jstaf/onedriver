package graph

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResourcePath(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		`/drives/me/root:%2Fsome%20path%2Fhere%21`,
		ResourcePath("me", "/some path/here!"),
		"Escaped path was wrong.",
	)
}

func TestRequestUnauthenticated(t *testing.T) {
	t.Parallel()
	badAuth := &Auth{
		// Set a renewal 1 year in the future so we don't accidentally overwrite
		// our auth tokens
		ExpiresAt: time.Now().Unix() + 60*60*24*365,
	}
	_, err := Get("/me/drive/root", badAuth)
	assert.Error(t, err, "An unauthenticated request was not handled as an error")
}
