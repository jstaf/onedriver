package systemd

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Chdir("../..")
	os.Mkdir("mount", 0700)
	os.Exit(m.Run())
}
