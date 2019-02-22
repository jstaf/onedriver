package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/jstaf/onedriver/graph"
	flag "github.com/spf13/pflag"
)

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

// A goroutine that will handle sigint then exit gracefully
func unmountHandler(signal <-chan os.Signal, server *fuse.Server) {
	sig := <-signal // block until sigint

	// signals don't automatically format well
	var code int
	var text string
	if sig == syscall.SIGINT {
		text = "SIGINT"
		code = int(syscall.SIGINT)
	} else {
		text = "SIGTERM"
		code = int(syscall.SIGTERM)
	}
	log.Println(text, "received, unmounting filesystem...")
	err := server.Unmount()
	if err != nil {
		log.Println(err)
	}

	// convention when exiting via signal is 128 + signal value
	os.Exit(128 + int(code))
}

func main() {
	// setup cli parsing
	authOnly := flag.BoolP("auth-only", "a", false,
		"Authenticate to Onedrive and then exit. Useful for running tests.")
	version := flag.BoolP("version", "v", false, "Display program version.")
	debugOn := flag.BoolP("debug", "d", false, "Enable FUSE debug logging.")
	flag.BoolP("help", "h", false, "Display usage and help.")
	flag.Usage = usage
	flag.Parse()

	if *version {
		fmt.Println("onedriver v0.1")
		os.Exit(0)
	}

	if *authOnly {
		// early quit if all we wanted to do was authenticate
		graph.Authenticate()
		os.Exit(0)
	}

	if len(flag.Args()) != 1 {
		// no mountpoint provided
		flag.Usage()
		os.Exit(1)
	}

	// setup filesystem
	fs := pathfs.NewPathNodeFs(graph.NewFS(), nil)
	server, _, err := nodefs.MountRoot(flag.Arg(0), fs.Root(), nil)
	if err != nil {
		log.Fatalf("Mount failed. Is the mountpoint already in use? "+
			"(Try running \"fusermount -u %s\")\n%v", flag.Arg(0), err)
	}
	server.SetDebug(*debugOn)

	// setup sigint handler for graceful unmount on interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go unmountHandler(sigChan, server)

	// serve filesystem
	server.Serve()
}
