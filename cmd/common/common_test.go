package common

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Write a sample .xdg-volume-info file and check that it can be read.
func TestXDGVolumeInfo(t *testing.T) {
	const expected = "some-volume name *()! $"
	content := TemplateXDGVolumeInfo(expected)
	file, _ := os.CreateTemp("", "onedriver-test-*")
	os.WriteFile(file.Name(), []byte(content), 0600)
	driveName, err := GetXDGVolumeInfoName(file.Name())
	require.NoError(t, err)
	assert.Equal(t, expected, driveName)
}
