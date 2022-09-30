//go:build linux && cgo
// +build linux,cgo

package ui

import (
	"os"

	"github.com/gotk3/gotk3/gtk"
)

// DirChooser is used to pick a directory
func DirChooser(title string) string {
	chooser, _ := gtk.FileChooserNativeDialogNew(title, nil,
		gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER, "Select", "Cancel")
	homedir, _ := os.UserHomeDir()
	chooser.SetCurrentFolder(homedir)

	var directory string
	chooser.Connect("response", func() {
		directory = chooser.GetFilename()
	})

	if chooser.Run() == int(gtk.RESPONSE_ACCEPT) {
		return directory
	}
	return ""
}

// Dialog creates a popup message
func Dialog(msg string, messageType gtk.MessageType, parentWindow gtk.IWindow) {
	messageDialog := gtk.MessageDialogNew(
		parentWindow,
		gtk.DIALOG_DESTROY_WITH_PARENT,
		messageType,
		gtk.BUTTONS_CLOSE,
		msg,
	)
	messageDialog.Run()
	messageDialog.Destroy()
}

// CancelDialog creates a "Continue?" style message, and returns what the user
// selected
func CancelDialog(title string, parentWindow gtk.IWindow) bool {
	dialog, _ := gtk.DialogNewWithButtons(title, parentWindow, gtk.DIALOG_MODAL,
		[]interface{}{"Cancel", gtk.RESPONSE_CANCEL},
		[]interface{}{"Continue", gtk.RESPONSE_ACCEPT},
	)
	defer dialog.Destroy()
	return dialog.Run() == gtk.RESPONSE_ACCEPT
}
