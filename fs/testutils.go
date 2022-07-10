package fs

// shared resources for special tests of this package (offline/ and shared/)

import (
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	MountLoc      = "mount"
	TestDir       = MountLoc + "/onedriver_tests"
	TestSharedDir = MountLoc + "/Shared with me"
)

// ChdirToProjectRoot fixes the working directory for tests
func ChdirToProjectRoot() {
	for wd, _ := os.Getwd(); !strings.HasSuffix(wd, "/onedriver"); wd, _ = os.Getwd() {
		// depending on how this test gets launched, the working directory can be wrong
		os.Chdir("..")
	}
}

// CleanupMountPoint cleans up the mountpoint before/after tests by unmounting any
// active drives there.
func CleanupMountpoint() {
	exec.Command("fusermount", "-uz", MountLoc).Run()
	os.Mkdir(MountLoc, 0755)
}

// SetupTestLogger initializes zerolog for testing
func SetupTestLogger() *os.File {
	f, _ := os.OpenFile("fusefs_tests.log", os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: f, TimeFormat: "15:04:05"})
	return f
}

// SetupTestFilesystem sets up a test filesystem
func SetupTestFilesystem(dbPath string, auth *graph.Auth) *Filesystem {
	filesystem := NewFilesystem(auth, dbPath)
	server, _ := fuse.NewServer(
		filesystem,
		MountLoc,
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
	go UnmountHandler(sigChan, server)

	// mount fs in background thread
	go server.Serve()
	return filesystem
}
