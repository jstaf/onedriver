package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	require.NoError(t, err)
	allDrives := make([]string, 0)
	for _, d := range drives {
		allDrives = append(allDrives, d.ID)
	}
	assert.Contains(t, allDrives, me.ID,
		"Could not find the \"me\" drive in all drives available to user.")
}
