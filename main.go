package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/jstaf/onedriver/graph"
	flag "github.com/spf13/pflag"
)

var auth graph.Auth

type fuseFs struct {
	pathfs.FileSystem
}

// these files will never exist, and we should ignore them
func ignore(path string) bool {
	ignoredFiles := []string{
		"/BDMV",
		"/.Trash",
		"/.Trash-1000",
		"/.xdg-volume-info",
		"/autorun.inf",
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
	item, err := graph.GetItem(name, auth)
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

func (fs *fuseFs) OpenDir(name string, context *fuse.Context) (c []fuse.DirEntry, code fuse.Status) {
	name = "/" + name
	log.Printf("OpenDir(\"%s\")", name)
	children, err := graph.GetChildren(name, auth)
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
	version := flag.BoolP("version", "v", false, "Display program version.")
	flag.BoolP("help", "h", false, "Display usage and help.")
	flag.Usage = usage
	flag.Parse()

	// act on flags
	if *version {
		fmt.Println("onedriver v0.1")
		os.Exit(1)
	}
	if *authOnly {
		graph.Authenticate()
		os.Exit(0)
	}
	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	auth = graph.Authenticate()

	fs := pathfs.NewPathNodeFs(&fuseFs{
		FileSystem: pathfs.NewDefaultFileSystem(),
	}, nil)
	server, _, err := nodefs.MountRoot(flag.Arg(0), fs.Root(), nil)
	if err != nil {
		log.Fatalf("Mount failed: %v\n", err)
	}
	server.Serve()
}
