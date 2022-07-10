package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog/log"
)

const (
	DeltaDir     = TestDir + "/delta"
	retrySeconds = 60 * time.Second //lint:ignore ST1011 a
)

// global variables required by tests to inject stuff into the filesystem and submit
// authenticated requests
var (
	auth *graph.Auth
	fs   *Filesystem
)

// Tests are done in the main project directory with a mounted filesystem to
// avoid having to repeatedly recreate auth_tokens.json and juggle multiple auth
// sessions.
func TestMain(m *testing.M) {
	// determine if we're running a single test in vscode or something
	var singleTest bool
	for _, arg := range os.Args {
		if strings.Contains(arg, "-test.run") {
			singleTest = true
		}
	}

	// attempt to unmount regardless of what happens (in case previous tests
	// failed and didn't clean themselves up)
	ChdirToProjectRoot()
	CleanupMountpoint()

	// wipe all cached data from previous tests
	toDelete, _ := filepath.Glob("test*.db")
	for _, db := range toDelete {
		os.Remove(db)
	}

	f := SetupTestLogger()
	defer f.Close()

	auth = graph.Authenticate(graph.AuthConfig{}, ".auth_tokens.json", false)
	fs = SetupTestFilesystem("test.db", auth)

	// cleanup from last run
	log.Info().Msg("Setup test environment ---------------------------------")
	if err := os.RemoveAll(TestDir); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Mkdir(TestDir, 0755)
	os.Mkdir(DeltaDir, 0755)

	// create paging test files before the delta thread is created
	if !singleTest {
		os.Mkdir(filepath.Join(TestDir, "paging"), 0755)
		createPagingTestFiles()
	}
	go fs.DeltaLoop(5 * time.Second)

	// not created by default on onedrive for business
	os.Mkdir(MountLoc+"/Documents", 0755)

	// we do not cd into the mounted directory or it will hang indefinitely on
	// unmount with "device or resource busy"

	// run tests
	log.Info().Msg("Test session start ---------------------------------")
	code := m.Run()
	log.Info().Msg("Test session end -----------------------------------")

	// cleanup
	fmt.Printf("Waiting 5 seconds for any remaining uploads to complete")
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		fmt.Printf(".")
	}
	fmt.Printf("\n")
	CleanupMountpoint()
	os.Exit(code)
}

// Apparently 200 reqests is the default paging limit.
// Upload at least this many for a later test before the delta thread is created.
func createPagingTestFiles() {
	fmt.Println("Setting up paging test files.")
	var group sync.WaitGroup
	var errCounter int64
	for i := 0; i < 250; i++ {
		group.Add(1)
		go func(n int, wg *sync.WaitGroup) {
			_, err := graph.Put(
				graph.ResourcePath("me", fmt.Sprintf("/onedriver_tests/paging/%d.txt", n))+":/content",
				auth,
				strings.NewReader("test\n"),
			)
			if err != nil {
				log.Error().Err(err).Msg("Paging upload fail.")
				atomic.AddInt64(&errCounter, 1)
			}
			wg.Done()
		}(i, &group)
	}
	group.Wait()
	log.Info().Msgf("%d failed paging uploads.\n", errCounter)
	fmt.Println("Finished with paging test setup.")
}
