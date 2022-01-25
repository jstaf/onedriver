package ui

import (
	"io/ioutil"
	"os"
	"testing"
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
		if MountpointIsValid(test.mountpoint) != test.expected {
			t.Errorf("Did not correctly determine if mountpoint \"%s\" was valid.\n",
				test.mountpoint)
		}
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
		if result := EscapeHome(test.unescaped); result != test.escaped {
			t.Errorf("Did not correctly escape home. Got \"%s\", wanted \"%s\"\n",
				result, test.escaped)
		}
		if result := UnescapeHome(test.escaped); result != test.unescaped {
			t.Errorf("Did not correctly escape home. Got \"%s\", wanted \"%s\"\n",
				result, test.unescaped)
		}
	}
}
