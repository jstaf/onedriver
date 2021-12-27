package systemd

import (
	"os"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	os.Chdir("../..")
	os.Mkdir("mount", 0700)
	os.Exit(m.Run())
}

// convenience handler to fail tests if an error is not nil
func failOnErr(t *testing.T, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		t.Logf("Test failed at %s:%d:\n", file, line)
		t.Fatal(err)
	}
}
