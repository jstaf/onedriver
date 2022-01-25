// common functions used by both binaries
package common

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const version = "0.12.0"

var commit string

// Version returns the current version string
func Version() string {
	clen := 0
	if len(commit) > 7 {
		clen = 8
	}
	return fmt.Sprintf("v%s %s", version, commit[:clen])
}

// StringToLevel converts a string to a LogLevel in a case-insensitive manner.
func StringToLevel(level string) zerolog.Level {
	level = strings.ToLower(level)
	switch level {
	case "fatal":
		return zerolog.FatalLevel
	case "error":
		return zerolog.ErrorLevel
	case "warn":
		return zerolog.WarnLevel
	case "info":
		return zerolog.InfoLevel
	case "debug":
		return zerolog.DebugLevel
	case "trace":
		return zerolog.TraceLevel
	default:
		log.Error().Msgf("Unrecognized log level \"%s\", defaulting to \"trace\".\n", level)
		return zerolog.TraceLevel
	}
}
