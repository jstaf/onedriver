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

	response := chooser.Run()
	if response == int(gtk.RESPONSE_ACCEPT) {
		return directory
	}
	return ""
}
