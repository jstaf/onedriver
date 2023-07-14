package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const configTestDir = "pkg/resources/test"

// We should load config correctly.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	conf := LoadConfig(filepath.Join(configTestDir, "config-test.yml"))

	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, "somewhere/else"), conf.CacheDir)
	assert.Equal(t, "warn", conf.LogLevel)
}

func TestConfigMerge(t *testing.T) {
	t.Parallel()

	conf := LoadConfig(filepath.Join(configTestDir, "config-test-merge.yml"))

	assert.Equal(t, "debug", conf.LogLevel)
	assert.Equal(t, "/some/directory", conf.CacheDir)
}

// We should come up with the defaults if there is no config file.
func TestLoadNonexistentConfig(t *testing.T) {
	t.Parallel()

	conf := LoadConfig(filepath.Join(configTestDir, "does-not-exist.yml"))

	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".cache/onedriver"), conf.CacheDir)
	assert.Equal(t, "debug", conf.LogLevel)
}
