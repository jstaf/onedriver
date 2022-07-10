package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/unit"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/cmd/common"
	"github.com/jstaf/onedriver/fs"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	flag "github.com/spf13/pflag"
)

func usage() {
	fmt.Printf(`onedriver - A Linux client for Microsoft OneDrive.

This program will mount your OneDrive account as a Linux filesystem at the
specified mountpoint. Note that this is not a sync client - files are only
fetched on-demand and cached locally. Only files you actually use will be
downloaded. While offline, the filesystem will be read-only until
connectivity is re-established.

Usage: onedriver [options] <mountpoint>

Valid options:
`)
	flag.PrintDefaults()
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})

	// setup cli parsing
	authOnly := flag.BoolP("auth-only", "a", false,
		"Authenticate to OneDrive and then exit.")
	headless := flag.BoolP("no-browser", "n", false,
		"This disables launching the built-in web browser during authentication. "+
			"Follow the instructions in the terminal to authenticate to OneDrive.")
	configPath := flag.StringP("config-file", "f", common.DefaultConfigPath(),
		"A YAML-formatted configuration file used by onedriver.")
	logLevel := flag.StringP("log", "l", "",
		"Set logging level/verbosity for the filesystem. "+
			"Can be one of: fatal, error, warn, info, debug, trace")
	cacheDir := flag.StringP("cache-dir", "c", "",
		"Change the default cache directory used by onedriver. "+
			"Will be created if the path does not already exist.")
	wipeCache := flag.BoolP("wipe-cache", "w", false,
		"Delete the existing onedriver cache directory and then exit. "+
			"This is equivalent to resetting the program.")
	versionFlag := flag.BoolP("version", "v", false, "Display program version.")
	debugOn := flag.BoolP("debug", "d", false, "Enable FUSE debug logging. "+
		"This logs communication between onedriver and the kernel.")
	help := flag.BoolP("help", "h", false, "Displays this help message.")
	flag.Usage = usage
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}
	if *versionFlag {
		fmt.Println("onedriver", common.Version())
		os.Exit(0)
	}

	config := common.LoadConfig(*configPath)
	// command line options override config options
	if *cacheDir != "" {
		config.CacheDir = *cacheDir
	}
	if *logLevel != "" {
		config.LogLevel = *logLevel
	}

	zerolog.SetGlobalLevel(common.StringToLevel(config.LogLevel))

	// wipe cache if desired
	if *wipeCache {
		log.Info().Str("path", config.CacheDir).Msg("Removing cache.")
		os.RemoveAll(config.CacheDir)
		os.Exit(0)
	}

	// determine and validate mountpoint
	if len(flag.Args()) == 0 {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "\nNo mountpoint provided, exiting.\n")
		os.Exit(1)
	}

	mountpoint := flag.Arg(0)
	st, err := os.Stat(mountpoint)
	if err != nil || !st.IsDir() {
		log.Fatal().
			Str("mountpoint", mountpoint).
			Msg("Mountpoint did not exist or was not a directory.")
	}
	if res, _ := ioutil.ReadDir(mountpoint); len(res) > 0 {
		log.Fatal().Str("mountpoint", mountpoint).Msg("Mountpoint must be empty.")
	}

	// compute cache name as systemd would
	absMountPath, _ := filepath.Abs(mountpoint)
	cachePath := filepath.Join(config.CacheDir, unit.UnitNamePathEscape(absMountPath))

	// authenticate/re-authenticate if necessary
	os.MkdirAll(cachePath, 0700)
	authPath := filepath.Join(cachePath, "auth_tokens.json")
	if *authOnly {
		os.Remove(authPath)
		graph.Authenticate(config.AuthConfig, authPath, *headless)
		os.Exit(0)
	}

	// create the filesystem
	log.Info().Msgf("onedriver %s", common.Version())
	auth := graph.Authenticate(config.AuthConfig, authPath, *headless)
	filesystem := fs.NewFilesystem(auth, filepath.Join(cachePath, "onedriver.db"))
	go filesystem.DeltaLoop(30 * time.Second)

	server, err := fuse.NewServer(filesystem, mountpoint, &fuse.MountOptions{
		Name:          "onedriver",
		FsName:        "onedriver",
		DisableXAttrs: true,
		MaxBackground: 1024,
		Debug:         *debugOn,
	})
	if err != nil {
		log.Fatal().Err(err).Msgf("Mount failed. Is the mountpoint already in use? "+
			"(Try running \"fusermount -uz %s\")\n", mountpoint)
	}

	// setup signal handler for graceful unmount on signals like sigint
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go fs.UnmountHandler(sigChan, server)

	// serve filesystem
	log.Info().
		Str("cachePath", cachePath).
		Str("mountpoint", absMountPath).
		Msg("Serving filesystem.")
	server.Serve()
}
