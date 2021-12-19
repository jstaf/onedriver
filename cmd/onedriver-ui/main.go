package main

import (
	"os"

	"github.com/coreos/go-systemd/v22/unit"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/jstaf/onedriver/cmd/common"
	"github.com/jstaf/onedriver/ui"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})

	log.Info().Msgf("onedriver-launcher %s", common.Version())

	app, err := gtk.ApplicationNew("com.github.jstaf.onedriver", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create application.")
	}
	app.Connect("activate", func(application *gtk.Application) {
		activateCallback(application)
	})
	app.Run(os.Args)
}

// activateCallback is what actually sets up the application
func activateCallback(app *gtk.Application) {
	window, _ := gtk.ApplicationWindowNew(app)
	window.SetDefaultSize(550, 400)

	header, _ := gtk.HeaderBarNew()
	header.SetShowCloseButton(true)
	header.SetTitle("onedriver")
	window.SetTitlebar(header)

	listbox, _ := gtk.ListBoxNew()
	window.Add(listbox)

	mountpointBtn, _ := gtk.ButtonNewFromIconName("list-add-symbolic", gtk.ICON_SIZE_BUTTON)
	mountpointBtn.SetTooltipText("Add a new OneDrive account.")
	mountpointBtn.Connect("clicked", func(button *gtk.Button) {
		mount := ui.DirChooser("Select a mountpoint")
		if !ui.MountpointIsValid(mount) {
			log.Error().Str("mountpoint", mount).
				Msg("Mountpoint was not valid. Mountpoint must be an empty directory.")

		}
		log.Info().Str("mountpoint", mount).Msg("Creating mountpoint.")
		escapedMount := unit.UnitNamePathEscape(mount)
		systemdUnit := ui.SystemdTemplateUnit(ui.OnedriverServiceTemplate, escapedMount)
	})
	header.PackStart(mountpointBtn)

	window.ShowAll()
}
