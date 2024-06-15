//go:build !linux || !cgo
// +build !linux !cgo

package main

import (
	"github.com/jstaf/onedriver/fs"
	"github.com/jstaf/onedriver/fs/graph"
)

func xdgVolumeInfo(filesystem *fs.Filesystem, auth *graph.Auth) {
	return
}
