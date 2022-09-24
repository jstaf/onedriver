// This package exists to allow testing shared items functionality independently of
// other tests (some accounts may not have shared items pre-populated yet).
package shared

import (
	"os"
	"testing"

	"github.com/jstaf/onedriver/fs"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog/log"
)

func TestMain(m *testing.M) {
	fs.ChdirToProjectRoot()
	fs.CleanupMountpoint()

	f := fs.SetupTestLogger()
	defer f.Close()
	log.Info().Msg("Setup shared tests ------------------------------")

	// reuses the cached data from the previous tests
	auth := graph.Authenticate(graph.AuthConfig{}, ".auth_tokens.json", false)
	fs.SetupTestFilesystem("test.db", auth)

	log.Info().Msg("Start shared tests ------------------------------")
	code := m.Run()
	log.Info().Msg("Finish shared tests ------------------------------")
	fs.CleanupMountpoint()
	os.Exit(code)
}
