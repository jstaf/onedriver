// This package exists purely for the convenience of easily running tests which
// test the offline functionality of the graph package.
// `unshare -nr` is used to deny network access, and then the tests are run using
// cached data from the tests in the graph package.
package offline

import (
	"fmt"
	"os"
	"testing"

	"github.com/jstaf/onedriver/fs"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog/log"
)

// Like the graph package, but designed for running tests offline.
func TestMain(m *testing.M) {
	fs.ChdirToProjectRoot()
	fs.CleanupMountpoint()

	auth := graph.Authenticate(graph.AuthConfig{}, ".auth_tokens.json", false)
	inode, err := graph.GetItem("me", "root", auth)
	if inode != nil || !graph.IsOffline(err) {
		fmt.Println("These tests must be run offline.")
		os.Exit(1)
	}

	f := fs.SetupTestLogger()
	defer f.Close()
	log.Info().Msg("Setup offline tests ------------------------------")

	// reuses the cached data from the previous tests
	fs.SetupTestFilesystem("test.db", auth)

	log.Info().Msg("Start offline tests ------------------------------")
	code := m.Run()
	log.Info().Msg("Finish offline tests ------------------------------")
	os.Exit(code)
	fs.CleanupMountpoint()
}
