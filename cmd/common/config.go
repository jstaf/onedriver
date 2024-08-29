package common

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"dario.cat/mergo"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/jstaf/onedriver/ui"
	"github.com/rs/zerolog/log"
	yaml "gopkg.in/yaml.v3"
)

type Config struct {
	CacheDir         string `yaml:"cacheDir"`
	LogLevel         string `yaml:"log"`
	graph.AuthConfig `yaml:"auth"`
}

// DefaultConfigPath returns the default config location for onedriver
func DefaultConfigPath() string {
	confDir, err := os.UserConfigDir()
	if err != nil {
		log.Error().Err(err).Msg("Could not determine configuration directory.")
	}
	return filepath.Join(confDir, "onedriver/config.yml")
}

// LoadConfig is the primary way of loading onedriver's config
func LoadConfig(path string) *Config {
	xdgCacheDir, _ := os.UserCacheDir()
	defaults := Config{
		CacheDir: filepath.Join(xdgCacheDir, "onedriver"),
		LogLevel: "debug",
	}

	conf, err := ioutil.ReadFile(path)
	if err != nil {
		log.Warn().
			Err(err).
			Str("path", path).
			Msg("Configuration file not found, using defaults.")
		return &defaults
	}
	config := &Config{}
	if err = yaml.Unmarshal(conf, config); err != nil {
		log.Error().
			Err(err).
			Str("path", path).
			Msg("Could not parse configuration file, using defaults.")
	}
	if err = mergo.Merge(config, defaults); err != nil {
		log.Error().
			Err(err).
			Str("path", path).
			Msg("Could not merge configuration file with defaults, using defaults only.")
	}

	config.CacheDir = ui.UnescapeHome(config.CacheDir)
	return config
}

// Write config to a file
func (c Config) WriteConfig(path string) error {
	out, err := yaml.Marshal(c)
	if err != nil {
		log.Error().Err(err).Msg("Could not marshal config!")
		return err
	}
	os.MkdirAll(filepath.Dir(path), 0700)
	err = ioutil.WriteFile(path, out, 0600)
	if err != nil {
		log.Error().Err(err).Msg("Could not write config to disk.")
	}
	return err
}
