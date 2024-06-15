//go:build linux && cgo
// +build linux,cgo

package main

import (
	"encoding/json"
	"strings"

	"github.com/jstaf/onedriver/fs"
	"github.com/jstaf/onedriver/cmd/common"
	"github.com/jstaf/onedriver/fs/graph"
	"github.com/rs/zerolog/log"
)

// xdgVolumeInfo createx .xdg-volume-info for a nice little onedrive logo in the
// corner of the mountpoint and shows the account name in the nautilus sidebar
func xdgVolumeInfo(filesystem *fs.Filesystem, auth *graph.Auth) {
	if child, _ := filesystem.GetPath("/.xdg-volume-info", auth); child != nil {
		return
	}
	log.Info().Msg("Creating .xdg-volume-info")
	user, err := graph.GetUser(auth)
	if err != nil {
		log.Error().Err(err).Msg("Could not create .xdg-volume-info")
		return
	}
	xdgVolumeInfo := common.TemplateXDGVolumeInfo(user.UserPrincipalName)

	// just upload directly and shove it in the cache
	// (since the fs isn't mounted yet)
	resp, err := graph.Put(
		graph.ResourcePath("/.xdg-volume-info")+":/content",
		auth,
		strings.NewReader(xdgVolumeInfo),
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to write .xdg-volume-info")
	}
	root, _ := filesystem.GetPath("/", auth) // cannot fail
	inode := fs.NewInode(".xdg-volume-info", 0644, root)
	if json.Unmarshal(resp, &inode) == nil {
		filesystem.InsertID(inode.ID(), inode)
	}
}
