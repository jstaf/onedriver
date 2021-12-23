package systemd

import (
	"os"
	"runtime"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestMain(m *testing.M) {
	os.Chdir("../..")
	f, _ := os.OpenFile("fusefs_tests.log", os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: f, TimeFormat: "15:04:05"})
	defer f.Close()

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
