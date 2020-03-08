package graph

import (
	"os"
	"strings"

	"github.com/hanwen/go-fuse/v2/fuse"
	log "github.com/sirupsen/logrus"
)

// UnmountHandler should be used as goroutine that will handle sigint then exit gracefully
func UnmountHandler(signal <-chan os.Signal, server *fuse.Server) {
	sig := <-signal // block until signal
	log.WithFields(log.Fields{
		"signal": strings.ToUpper(sig.String()),
	}).Info("Signal received, unmounting filesystem.")

	err := server.Unmount()
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Failed to unmount filesystem cleanly!")
	}

	os.Exit(128)
}
