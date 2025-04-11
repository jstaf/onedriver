//go:build linux && cgo
// +build linux,cgo

package ui

import (
    "os"
    "context"

    "github.com/diamondburned/gotk4/pkg/gtk/v4"
    "github.com/diamondburned/gotk4/pkg/glib/v2"
)

// DirChooser is used to pick a directory with better error handling
func DirChooser(title string, parent *gtk.Window) string {
    chooser := gtk.NewFileChooserNative(
        title, 
        parent,
        gtk.FileChooserActionSelectFolder, 
        "Select", 
        "Cancel",
    )
    
    homedir, err := os.UserHomeDir()
    if err == nil {
        chooser.SetCurrentFolder(glib.NewFilePath(homedir))
    }

    var directory string
    chooser.Connect("response", func() {
        directory = chooser.GetFile().GetPath()
    })

    if chooser.Run() == int(gtk.ResponseAccept) {
        return directory
    }
    return ""
}

// Dialog creates a popup message
func Dialog(msg string, messageType gtk.MessageType, parentWindow gtk.IWindow) {
    messageDialog := gtk.NewMessageDialog(
        parentWindow,
        gtk.DialogDestroyWithParent,
        messageType,
        gtk.ButtonsClose,
        msg,
    )
    messageDialog.Run()
    messageDialog.Destroy()
}

// CancelDialog creates a "Continue?" style message, and returns what the user
// selected
func CancelDialog(parentWindow gtk.IWindow, primaryText, secondaryText string) bool {
    dialog := gtk.NewMessageDialog(
        parentWindow,
        gtk.DialogModal,
        gtk.MessageWarning,
        gtk.ButtonsOkCancel,
        "",
    )
    dialog.SetMarkup(primaryText)
    if secondaryText != "" {
        dialog.FormatSecondaryMarkup(secondaryText)
    }
    defer dialog.Destroy()
    return dialog.Run() == gtk.ResponseOk
}
