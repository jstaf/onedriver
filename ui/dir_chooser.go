package ui

import (
	"os"

	"github.com/gotk3/gotk3/gtk"
	"github.com/rs/zerolog/log"
)

func DirChooser(title string) string {
	gtk.Init(nil)
	chooser, _ := gtk.FileChooserNativeDialogNew(title, nil, gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER, "Select", "")
	homedir, _ := os.UserHomeDir()
	chooser.SetCurrentFolder(homedir)

	chooser.Connect("response", func() {
		log.Info().Msg("test")
	})

	chooser.Show()
	gtk.Main()
	return ""
}
