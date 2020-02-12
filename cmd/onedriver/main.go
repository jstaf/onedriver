package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jstaf/onedriver/graph"
	"github.com/jstaf/onedriver/logger"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

const onedriverVersion = "0.7.0"

func usage() {
	fmt.Printf(`onedriver - A Linux client for Onedrive.

This program will mount your Onedrive account as a Linux filesystem at the
specified mountpoint. Note that this is not a sync client - files are fetched
on-demand and cached locally. Only files you actually use will be downloaded.
This filesystem requires an active internet connection to work.

Usage: onedriver [options] <mountpoint>

Valid options:
`)
	flag.PrintDefaults()
}

func main() {
	// setup cli parsing
	authOnly := flag.BoolP("auth-only", "a", false,
		"Authenticate to Onedrive and then exit. Useful for running tests.")
	logLevel := flag.String("log", "debug", "Set logging level/verbosity. "+
		"Can be one of: fatal, error, warn, info, debug, trace")
	cacheDir := flag.StringP("cache-dir", "c", "",
		"Change the default cache directory used by onedriver. "+
			"Will be created if the path does not already exist.")
	wipeCache := flag.BoolP("wipe-cache", "w", false,
		"Delete the existing onedriver cache directory.")
	version := flag.BoolP("version", "v", false, "Display program version.")
	debugOn := flag.BoolP("debug", "d", false, "Enable FUSE debug logging.")
	flag.BoolP("help", "h", false, "Display usage and help.")
	flag.Usage = usage
	flag.Parse()

	if *version {
		fmt.Println("onedriver v" + onedriverVersion)
		os.Exit(0)
	}

	dir := *cacheDir
	if dir == "" {
		dir = graph.CacheDir()
	}

	if *authOnly {
		// early quit if all we wanted to do was authenticate
		graph.Authenticate(filepath.Join(dir, "auth_tokens.json"))
		os.Exit(0)
	}

	log.SetLevel(logger.StringToLevel(*logLevel))
	log.SetReportCaller(true)
	log.SetFormatter(logger.LogrusFormatter())

	if len(flag.Args()) != 1 {
		// no mountpoint provided
		flag.Usage()
		os.Exit(1)
	}

	if *wipeCache {
		os.RemoveAll(dir)
	}

	log.Info("onedriver v", onedriverVersion)

	// setup filesystem
	if st, _ := os.Stat(dir); st == nil {
		os.Mkdir(dir, 0700)
	}

	root := graph.NewFS(
		filepath.Join(dir, "onedriver.db"),
		filepath.Join(dir, "auth_tokens.json"),
		30*time.Second,
	)

	// Create .xdg-volume-info for a nice little onedrive logo in the corner of the
	// mountpoint and show the account name in the nautilus sidebar
	cache := root.GetCache()
	auth := cache.GetAuth()
	if child, _ := cache.GetPath("/.xdg-volume-info", auth); child == nil {
		log.Info("Creating .xdg-volume-info")
		user, err := graph.GetUser(auth)
		if err != nil {
			log.Error("Could not create .xdg-volume-info: ", err)
		} else {
			xdgVolumeInfo := fmt.Sprintf("[Volume Info]\nName=%s\n", user.UserPrincipalName)
			if _, err := os.Stat("/usr/share/icons/onedriver.png"); err == nil {
				xdgVolumeInfo += "IconFile=/usr/share/icons/onedriver.png\n"
			}
			// just upload directly and shove it in the cache
			// (since the fs isn't mounted yet)
			resp, err := graph.Put(
				graph.ResourcePath("/.xdg-volume-info")+":/content",
				auth,
				strings.NewReader(xdgVolumeInfo),
			)
			if err != nil {
				log.Error(err)
			}
			inode := graph.NewInode(".xdg-volume-info", 0644, root)
			err = json.Unmarshal(resp, &inode)
			if err == nil {
				cache.InsertID(inode.ID(), inode)
			}
		}
	}

	second := time.Second
	server, err := fs.Mount(flag.Arg(0), root, &fs.Options{
		EntryTimeout: &second,
		AttrTimeout:  &second,
		MountOptions: fuse.MountOptions{
			Name:          "onedriver",
			FsName:        "onedriver",
			DisableXAttrs: true,
			MaxBackground: 1024,
		},
	})
	if err != nil {
		log.Error(err)
		log.Fatalf("Mount failed. Is the mountpoint already in use? "+
			"(Try running \"fusermount -u %s\")\n", flag.Arg(0))
	}
	server.SetDebug(*debugOn)

	// setup sigint handler for graceful unmount on interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go graph.UnmountHandler(sigChan, server)

	// serve filesystem
	server.Wait()
}
