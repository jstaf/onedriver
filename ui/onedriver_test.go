package ui

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Can we detect a mountpoint as valid appropriately?
func TestMountpointIsValid(t *testing.T) {
	os.Mkdir("_test", 0755)
	ioutil.WriteFile("_test/.example", []byte("some text\n"), 0644)
	tests := []struct {
		mountpoint string
		expected   bool
	}{
		{"", false},
		{"fs", false},
		{"does_not_exist", false},
		{"mount", true},
		{"_test", false},
		{"_test/.example", false},
	}
	for _, test := range tests {
		assert.Equalf(t, test.expected, MountpointIsValid(test.mountpoint),
			"Did not correctly determine if mountpoint \"%s\" was valid.\n",
			test.mountpoint,
		)
	}

	os.RemoveAll("_test")
}

// Can we convert paths from ~/some_path to /home/username/some_path and back?
func TestHomeEscapeUnescape(t *testing.T) {
	homedir, _ := os.UserHomeDir()
	tests := []struct {
		unescaped, escaped string
	}{
		{homedir + "/test", "~/test"},
		{"/opt/test", "/opt/test"},
		{"/opt/test/~test.lock#", "/opt/test/~test.lock#"},
	}

	for _, test := range tests {
		assert.Equal(t, test.escaped, EscapeHome(test.unescaped),
			"Did not correctly escape home.")
		assert.Equal(t, test.unescaped, UnescapeHome(test.escaped),
			"Did not correctly unescape home.")
	}
}
