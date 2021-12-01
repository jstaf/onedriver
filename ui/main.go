package main

import (
	"os"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	app, err := gtk.ApplicationNew("com.github.jstaf.onedriver", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create application.")
	}
	app.Connect("activate", activate)
	app.Run(os.Args)
}

// activate is what actually sets up the application
func activate(app *gtk.Application) {
	window, err := gtk.ApplicationWindowNew(app)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create window.")
	}
	window.SetDefaultSize(550, 400)

	header, err := gtk.HeaderBarNew()
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create header bar")
	}
	header.SetShowCloseButton(true)
	header.SetTitle("onedriver")
	window.SetTitlebar(header)

	listbox, err := gtk.ListBoxNew()
	window.Container.Add(listbox)

	window.ShowAll()
}
