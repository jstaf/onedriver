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
func CancelDialog(parentWindow gtk.IWindow, primaryText, secondaryText string) bool {
	dialog := gtk.MessageDialogNew(
		parentWindow,
		gtk.DIALOG_MODAL,
		gtk.MESSAGE_WARNING,
		gtk.BUTTONS_OK_CANCEL,
		"",
	)
	dialog.SetMarkup(primaryText)
	if secondaryText != "" {
		dialog.FormatSecondaryMarkup(secondaryText)
	}
	defer dialog.Destroy()
	return dialog.Run() == gtk.RESPONSE_OK
}
