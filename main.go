package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	odfs "github.com/jstaf/onedriver/fs"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/jstaf/onedriver/logger"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

const version = "0.11.0"

var commit string

func usage() {
	fmt.Printf(`onedriver - A Linux client for Microsoft OneDrive.

This program will mount your OneDrive account as a Linux filesystem at the
specified mountpoint. Note that this is not a sync client - files are fetched
on-demand and cached locally. Only files you actually use will be downloaded.
While offline, the filesystem will be read-only until connectivity is re-
established.

Usage: onedriver [options] <mountpoint>

Valid options:
`)
	flag.PrintDefaults()
}

func main() {
	// setup cli parsing
	authOnly := flag.BoolP("auth-only", "a", false,
		"Authenticate to OneDrive and then exit.")
	logLevel := flag.StringP("log", "l", "debug", "Set logging level/verbosity. "+
		"Can be one of: fatal, error, warn, info, debug, trace")
	cacheDir := flag.StringP("cache-dir", "c", "",
		"Change the default cache directory used by onedriver. "+
			"Will be created if the path does not already exist.")
	wipeCache := flag.BoolP("wipe-cache", "w", false,
		"Delete the existing onedriver cache directory and then exit. "+
			"Equivalent to resetting the program.")
	versionFlag := flag.BoolP("version", "v", false, "Display program version.")
	debugOn := flag.BoolP("debug", "d", false, "Enable FUSE debug logging.")
	flag.BoolP("help", "h", false, "Displays this help message.")
	flag.Usage = usage
	flag.Parse()

	clen := 0
	if len(commit) > 7 {
		clen = 8
	}
	if *versionFlag {
		fmt.Printf("onedriver v%s %s\n", version, commit[:clen])
		os.Exit(0)
	}

	// determine cache directory and wipe if desired
	dir := *cacheDir
	if dir == "" {
		xdgCacheDir, _ := os.UserCacheDir()
		dir = filepath.Join(xdgCacheDir, "onedriver")
	}
	if *wipeCache {
		os.RemoveAll(dir)
		os.Exit(0)
	}

	// authenticate/re-authenticate if necessary
	os.MkdirAll(dir, 0700)
	authPath := filepath.Join(dir, "auth_tokens.json")
	if *authOnly {
		os.Remove(authPath)
		graph.Authenticate(authPath)
		os.Exit(0)
	}

	log.SetLevel(logger.StringToLevel(*logLevel))
	log.SetReportCaller(true)
	log.SetFormatter(logger.LogrusFormatter())

	// determine and validate mountpoint
	if len(flag.Args()) == 0 {
		flag.Usage()
		fmt.Printf("\nNo mountpoint provided, exiting.\n")
		os.Exit(1)
	}

	log.Infof("onedriver v%s %s", version, commit[:clen])
	mountpoint := flag.Arg(0)
	st, err := os.Stat(mountpoint)
	if err != nil || !st.IsDir() {
		log.WithField(
			"mountpoint", mountpoint,
		).Fatal("Mountpoint did not exist or was not a directory.")
	}
	if res, _ := ioutil.ReadDir(mountpoint); len(res) > 0 {
		log.WithField(
			"mountpoint", mountpoint,
		).Fatal("Mountpoint must be empty.")
	}

	// create a new filesystem and mount it
	auth := graph.Authenticate(authPath)
	cache := odfs.NewCache(auth, filepath.Join(dir, "onedriver.db"))
	root, _ := cache.GetPath("/", auth)
	go cache.DeltaLoop(30 * time.Second)

	xdgVolumeInfo(cache, auth)

	second := time.Second
	server, err := fs.Mount(mountpoint, root, &fs.Options{
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
		log.WithField("err", err).Fatalf("Mount failed. Is the mountpoint already in use? "+
			"(Try running \"fusermount -uz %s\")\n", mountpoint)
	}

	// setup signal handler for graceful unmount on signals like sigint
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go odfs.UnmountHandler(sigChan, server)

	// serve filesystem
	server.SetDebug(*debugOn)
	server.Wait()
}

// xdgVolumeInfo createx .xdg-volume-info for a nice little onedrive logo in the
// corner of the mountpoint and shows the account name in the nautilus sidebar
func xdgVolumeInfo(cache *odfs.Cache, auth *graph.Auth) {
	if child, _ := cache.GetPath("/.xdg-volume-info", auth); child != nil {
		return
	}
	log.Info("Creating .xdg-volume-info")
	user, err := graph.GetUser(auth)
	if err != nil {
		log.WithField("err", err).Error("Could not create .xdg-volume-info")
		return
	}

	xdgVolumeInfo := fmt.Sprintf("[Volume Info]\nName=%s\n", user.UserPrincipalName)
	if _, err := os.Stat("/usr/share/icons/onedriver/onedriver.png"); err == nil {
		xdgVolumeInfo += "IconFile=/usr/share/icons/onedriver/onedriver.png\n"
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
	root, _ := cache.GetPath("/", auth) // cannot fail
	inode := odfs.NewInode(".xdg-volume-info", 0644, root)
	if json.Unmarshal(resp, &inode) == nil {
		cache.InsertID(inode.ID(), inode)
	}
}
