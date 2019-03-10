package graph

import (
	"os"
	"testing"

	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

const (
	mountLoc = "mount"
	TestDir  = mountLoc + "/test"
)

var auth Auth

// Tests are done in the main project directory with a mounted filesystem to
// avoid having to repeatedly recreate auth_tokens.json and juggle mutliple auth
// sessions.
func TestMain(m *testing.M) {
	os.Chdir("..")
	os.Mkdir(mountLoc, 0755)

	fusefs := NewFS()
	auth = fusefs.Auth
	fs := pathfs.NewPathNodeFs(fusefs, nil)
	server, _, _ := nodefs.MountRoot(mountLoc, fs.Root(), nil)

	// mount fs in background thread
	go server.Serve()
	defer server.Unmount()

	os.Exit(m.Run())
}
