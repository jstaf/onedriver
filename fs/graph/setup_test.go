package graph

import (
	"os"
	"testing"

	"github.com/jstaf/onedriver/logger"
)

func TestMain(m *testing.M) {
	os.Chdir("../..")
	Authenticate(".auth_tokens.json")
	f := logger.LogTestSetup()
	defer f.Close()
	os.Exit(m.Run())
}
