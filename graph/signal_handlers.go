package graph

import (
	"os"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	log "github.com/sirupsen/logrus"
)

// UnmountHandler should be used as goroutine that will handle sigint then exit gracefully
func UnmountHandler(signal <-chan os.Signal, server *fuse.Server) {
	sig := <-signal // block until sigint

	// signals don't automatically format well
	var code int
	var text string
	if sig == syscall.SIGINT {
		text = "SIGINT"
		code = int(syscall.SIGINT)
	} else {
		text = "SIGTERM"
		code = int(syscall.SIGTERM)
	}
	log.Info(text, " received, unmounting filesystem.")
	err := server.Unmount()
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Failed to unmount filesystem cleanly!")
	}

	// convention when exiting via signal is 128 + signal value
	os.Exit(128 + int(code))
}
