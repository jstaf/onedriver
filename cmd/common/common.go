// common functions used by both binaries
package common

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const version = "0.15.0"

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

// TemplateXDGVolumeInfo returns
func TemplateXDGVolumeInfo(name string) string {
	xdgVolumeInfo := fmt.Sprintf("[Volume Info]\nName=%s\n", name)
	if _, err := os.Stat("/usr/share/icons/onedriver/onedriver.png"); err == nil {
		xdgVolumeInfo += "IconFile=/usr/share/icons/onedriver/onedriver.png\n"
	}
	return xdgVolumeInfo
}

// GetXDGVolumeInfoName returns the name of the drive according to whatever the
// user has named it.
func GetXDGVolumeInfoName(path string) (string, error) {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	regex := regexp.MustCompile("Name=(.*)")
	name := regex.FindString(string(contents))
	if len(name) < 5 {
		return "", errors.New("could not find \"Name=\" key")
	}
	return name[5:], nil
}
