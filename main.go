package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/jstaf/onedriver/graph"
	"github.com/jstaf/onedriver/logger"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

const onedriverVersion = "0.2"

func usage() {
	fmt.Printf(`onedriver - A Linux client for Onedrive.

This program will mount your Onedrive account as a Linux filesystem at the
specified mountpoint. Note that this is not a sync client - files are fetched
on-demand and cached locally. Only files you actually use will be downloaded.

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
		"Can be one of: fatal, error, warn, info, trace")
	version := flag.BoolP("version", "v", false, "Display program version.")
	debugOn := flag.BoolP("debug", "d", false, "Enable FUSE debug logging.")
	flag.BoolP("help", "h", false, "Display usage and help.")
	flag.Usage = usage
	flag.Parse()

	if *version {
		fmt.Println("onedriver v" + onedriverVersion)
		os.Exit(0)
	}

	if *authOnly {
		// early quit if all we wanted to do was authenticate
		graph.Authenticate()
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

	log.Info("onedriver v", onedriverVersion)

	// setup filesystem
	fs := pathfs.NewPathNodeFs(graph.NewFS("onedriver.db"), nil)
	server, _, err := nodefs.MountRoot(flag.Arg(0), fs.Root(), nil)
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
	server.Serve()
}
