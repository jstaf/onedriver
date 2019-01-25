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

type fuseFs struct {
	pathfs.FileSystem
	Auth graph.Auth
}

// these files will never exist, and we should ignore them
func ignore(path string) bool {
	ignoredFiles := []string{
		"/BDMV",
		"/.Trash",
		"/.Trash-1000",
		"/.xdg-volume-info",
		"/autorun.inf",
		"/.localized",
		"/.DS_Store",
		"/._.",
		"/.hidden",
	}
	for _, ignore := range ignoredFiles {
		if path == ignore {
			return true
		}
	}
	return false
}

func (fs *fuseFs) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	name = "/" + name
	if ignore(name) {
		return nil, fuse.ENOENT
	}
	log.Printf("GetAttr(\"%s\")", name)
	item, err := graph.GetItem(name, fs.Auth)
	if err != nil {
		return nil, fuse.ENOENT
	}

	// convert to UNIX struct stat
	attr := fuse.Attr{
		Size:  item.FakeSize(),
		Atime: item.MTime(),
		Mtime: item.MTime(),
		Ctime: item.MTime(),
		Mode:  item.Mode(),
		Owner: fuse.Owner{
			Uid: uint32(os.Getuid()),
			Gid: uint32(os.Getgid()),
		},
	}
	return &attr, fuse.OK
}

// not a valid option, since fuse is single-user anyways
func (fs *fuseFs) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

// no way to change mode yet
func (fs *fuseFs) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *fuseFs) OpenDir(name string, context *fuse.Context) (c []fuse.DirEntry, code fuse.Status) {
	name = "/" + name
	log.Printf("OpenDir(\"%s\")", name)
	children, err := graph.GetChildren(name, fs.Auth)
	if err != nil {
		// that directory probably doesn't exist. silly human.
		return nil, fuse.ENOENT
	}
	for _, child := range children {
		entry := fuse.DirEntry{
			Name: child.Name,
			Mode: child.Mode(),
		}
		c = append(c, entry)
	}
	return c, fuse.OK
}

func (fs *fuseFs) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	name = "/" + name
	item, err := graph.GetItem(name, fs.Auth)
	if err != nil {
		// doesn't exist or internet is out - either way, no files for you!
		return nil, fuse.ENOENT
	}

	//TODO deny write permissions until uploads/writes are implemented
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	body, err := graph.Get("/me/drive/items/"+item.ID+"/content", fs.Auth)
	if err != nil {
		log.Printf("Failed to fetch content for '%s': %s\n", item.ID, err)
		return nil, fuse.ENOENT
	}
	//TODO this is a read-only file - will need to implement our own version of
	// the File interface for write functionality
	return nodefs.NewDataFile(body), fuse.OK
}

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
	fs := pathfs.NewPathNodeFs(
		&fuseFs{
			FileSystem: pathfs.NewDefaultFileSystem(),
			Auth:       graph.Authenticate(),
		},
		nil)
	server, _, err := nodefs.MountRoot(flag.Arg(0), fs.Root(), nil)
	if err != nil {
		log.Fatalf("Mount failed. Is the mountpoint already in use?\n%v", err)
	}
	server.SetDebug(*debugOn)

	// setup sigint handler for graceful unmount on interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go unmountHandler(sigChan, server)

	// serve filesystem
	server.Serve()
}
