package offline

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	mountLoc = "mount"
	TestDir  = mountLoc + "/onedriver_tests"
)

var auth *graph.Auth

// Like the graph package, but designed for running tests offline.
func TestMain(m *testing.M) {
	if wd, _ := os.Getwd(); strings.HasSuffix(wd, "/offline") {
		// depending on how this test gets launched, the working directory can be wrong
		os.Chdir("../..")
	}

	// attempt to unmount regardless of what happens (in case previous tests
	// failed and didn't clean themselves up)
	exec.Command("fusermount", "-uz", mountLoc).Run()
	os.Mkdir(mountLoc, 0755)

	auth = graph.Authenticate(graph.AuthConfig{}, ".auth_tokens.json", false)
	inode, err := graph.GetItem("me", "root", auth)
	if inode != nil || !graph.IsOffline(err) {
		fmt.Println("These tests must be run offline.")
		os.Exit(1)
	}

	f, _ := os.OpenFile("fusefs_tests.log", os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: f, TimeFormat: "15:04:05"})
	defer f.Close()
	log.Info().Msg("Setup offline tests ------------------------------")

	// reuses the cached data from the previous tests
	server, _ := fuse.NewServer(
		fs.NewFilesystem(auth, "test.db"),
		mountLoc,
		&fuse.MountOptions{
			Name:          "onedriver",
			FsName:        "onedriver",
			DisableXAttrs: true,
			MaxBackground: 1024,
		},
	)

	// setup sigint handler for graceful unmount on interrupt/terminate
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT)
	go fs.UnmountHandler(sigChan, server)

	// mount fs in background thread
	go server.Serve()

	log.Info().Msg("Start offline tests ------------------------------")
	code := m.Run()
	log.Info().Msg("Finish offline tests ------------------------------")

	if server.Unmount() != nil {
		log.Error().Msg("Failed to unmount test fuse server, attempting lazy unmount")
		exec.Command("fusermount", "-zu", "mount").Run()
	}
	fmt.Println("Successfully unmounted fuse server!")
	os.Exit(code)
}
