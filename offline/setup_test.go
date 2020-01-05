package offline

import (
	"fmt"
	"os"
	"testing"

	"github.com/jstaf/onedriver/graph"
)

func TestMain(m *testing.M) {
	os.Chdir("..")

	auth := graph.Authenticate()
	inode, err := graph.GetItem("root", auth)
	if inode != nil || !graph.IsOffline(err) {
		fmt.Println("These tests must be run offline.")
		os.Exit(1)
	}

	os.Exit(m.Run())
}
