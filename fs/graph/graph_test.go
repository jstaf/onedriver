package graph

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourcePath(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		`/me/drive/root:%2Fsome%20path%2Fhere%21`,
		ResourcePath("/some path/here!"),
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

func TestDrives(t *testing.T) {
	// this is our default drive
	me, err := GetDrive("me", auth)
	require.NoError(t, err)

	// we should be able to fetch it by ID as well
	drive, err := GetDrive(me.ID, auth)
	require.NoError(t, err)

	assert.Equal(t, drive.ID, me.ID,
		"Me drive fetched using alternate methods should have same ID.")

	drives, err := GetAllDrives(auth)
	allDrives := make([]string, 0)
	for _, d := range drives {
		allDrives = append(allDrives, d.ID)
	}
	assert.Contains(t, allDrives, me.ID,
		"Could not find the \"me\" drive in all drives available to user.")
}
