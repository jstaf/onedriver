// common functions used by both binaries
package common

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const version = "0.14.1"

var commit string

// Version returns the current version string
func Version() string {
	clen := 0
	if len(commit) > 7 {
		clen = 8
	}
	return fmt.Sprintf("v%s %s", version, commit[:clen])
}

// StringToLevel converts a string to a zerolog.LogLevel that can be used with zerolog
func StringToLevel(input string) zerolog.Level {
	level, err := zerolog.ParseLevel(input)
	if err != nil {
		log.Error().Err(err).Msg("Could not parse log level, defaulting to \"debug\"")
		return zerolog.DebugLevel
	}
	return level
}

// LogLevels returns the available logging levels
func LogLevels() []string {
	return []string{"trace", "debug", "info", "warn", "error", "fatal"}
}
