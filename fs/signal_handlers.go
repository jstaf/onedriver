package fs

import (
	"os"
	"strings"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/rs/zerolog/log"
)

// UnmountHandler should be used as goroutine that will handle sigint then exit gracefully
func UnmountHandler(signal <-chan os.Signal, server *fuse.Server) {
	sig := <-signal // block until signal
	log.Info().Str("signal", strings.ToUpper(sig.String())).
		Msg("Signal received, unmounting filesystem.")

	err := server.Unmount()
	if err != nil {
		log.Error().Err(err).Msg("Failed to unmount filesystem cleanly! " +
			"Run \"fusermount3 -uz /MOUNTPOINT/GOES/HERE\" to unmount.")
	}

	os.Exit(128)
}
