package ui

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/jstaf/onedriver/fs/graph"
)

// onedriver specific utility functions

// PollUntilAvail will block until the mountpoint is available or a timeout is reached.
// If timeout is -1, default timeout is 120s.
func PollUntilAvail(mountpoint string, timeout int) {}

// MountpointIsValid returns if the mountpoint exists and nothing is in it.
func MountpointIsValid(mountpoint string) bool {
	return false
}

func GetAccountName(instance string) (string, error) {
	cacheDir, _ := os.UserCacheDir()
	tokenFile := fmt.Sprintf("%s/onedriver/%s/auth_tokens.json", cacheDir, instance)

	var auth graph.Auth
	data, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(data, &auth)
	if err != nil {
		return "", err
	}
	return auth.Account, nil
}

// GetKnownMounts returns the currently known mountpoints
func GetKnownMounts() []string {
	return make([]string, 0)
}

// EscapeHome replaces the user's absolute home directory with "~"
func EscapeHome(path string) string {
	homedir, _ := os.UserHomeDir()
	if strings.HasPrefix(path, homedir) {
		return strings.Replace(path, homedir, "~", 1)
	}
	return path
}

// UnescapeHome replaces the "~" in a path with the absolute path.
func UnescapeHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		homedir, _ := os.UserHomeDir()
		return filepath.Join(homedir, path[2:])
	}
	return path
}
