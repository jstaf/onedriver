// common functions used by both binaries
package common

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const version = "0.13.0"

var commit string

// Version returns the current version string
func Version() string {
	clen := 0
	if len(commit) > 7 {
		clen = 8
	}
	return fmt.Sprintf("v%s %s", version, commit[:clen])
}

func StringToLevel(input string) zerolog.Level {
	level, err := zerolog.ParseLevel(input)
	if err != nil {
		log.Error().Err(err).Msg("Could not parse log level, defaulting to \"debug\"")
		return zerolog.DebugLevel
	}
	return level
}
