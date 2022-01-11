package main

/*
#cgo linux pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/coreos/go-systemd/v22/unit"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/jstaf/onedriver/cmd/common"
	"github.com/jstaf/onedriver/ui"
	"github.com/jstaf/onedriver/ui/systemd"
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
	os.Exit(app.Run(os.Args))
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
			return
		}

		escapedMount := unit.UnitNamePathEscape(mount)
		systemdUnit := systemd.TemplateUnit(systemd.OnedriverServiceTemplate, escapedMount)
		log.Info().
			Str("mountpoint", mount).
			Str("systemdUnit", systemdUnit).
			Msg("Creating mountpoint.")

		if err := systemd.UnitSetActive(systemdUnit, true); err != nil {
			log.Error().Err(err).Msg("Failed to start unit.")
			return
		}

		// open it in default file browser
		cURI := C.CString("file://" + mount)
		C.g_app_info_launch_default_for_uri(cURI, nil, nil)
		C.free(unsafe.Pointer(cURI))

		row := newMountRow(mount)
		listbox.Insert(row, -1)
		row.ShowAll()
	})
	header.PackStart(mountpointBtn)

	window.ShowAll()
}

// newMountRow constructs a new ListBoxRow with the controls for an individual mountpoint.
func newMountRow(mount string) *gtk.ListBoxRow {
	row, _ := gtk.ListBoxRowNew()
	row.SetSelectable(true)
	box, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	row.Add(box)

	escapedMount := unit.UnitNamePathEscape(mount)
	unitName := systemd.TemplateUnit(systemd.OnedriverServiceTemplate, escapedMount)

	var label *gtk.Label
	tildePath := ui.EscapeHome(mount)
	accountName, err := ui.GetAccountName(escapedMount)
	if err != nil {
		log.Error().
			Err(err).
			Str("mountpoint", mount).
			Msg("Could not determine acccount name.")
		label, _ = gtk.LabelNew(tildePath)
	} else {
		label, _ = gtk.LabelNew("")
		label.SetMarkup(fmt.Sprintf("%s <span style=\"italic\" weight=\"light\">(%s)</span>    ",
			accountName, tildePath,
		))
	}
	box.PackStart(label, false, false, 5)

	// create a button to delete the mountpoint
	deleteMountpointBtn, _ := gtk.ButtonNewFromIconName("user-trash-symbolic", gtk.ICON_SIZE_BUTTON)
	deleteMountpointBtn.SetTooltipText("Remove OneDrive account from local computer")
	deleteMountpointBtn.Connect("clicked", func() {
		dialog, _ := gtk.DialogNewWithButtons("Remove mountpoint?", nil, gtk.DIALOG_MODAL,
			[]interface{}{"Cancel", gtk.RESPONSE_REJECT},
			[]interface{}{"Remove", gtk.RESPONSE_ACCEPT},
		)
		if dialog.Run() == gtk.RESPONSE_ACCEPT {
			systemd.UnitSetEnabled(unitName, false)
			systemd.UnitSetActive(unitName, false)

			cachedir, _ := os.UserCacheDir()
			os.RemoveAll(fmt.Sprintf("%s/onedriver/%s/", cachedir, escapedMount))

			row.Destroy()
		}
		dialog.Destroy()
	})
	box.PackEnd(deleteMountpointBtn, false, false, 0)

	// create a button to enable/disable the mountpoint
	unitEnabledBtn, _ := gtk.ToggleButtonNew()
	enabledImg, _ := gtk.ImageNewFromIconName("object-select-symbolic", gtk.ICON_SIZE_BUTTON)
	unitEnabledBtn.SetImage(enabledImg)
	unitEnabledBtn.SetTooltipText("Start mountpoint on login")
	enabled, err := systemd.UnitIsEnabled(unitName)
	if err == nil {
		unitEnabledBtn.SetActive(enabled)
	} else {
		log.Error().Err(err).Msg("Error checking unit enabled state.")
	}
	unitEnabledBtn.Connect("toggled", func() {
		err := systemd.UnitSetEnabled(unitName, unitEnabledBtn.GetActive())
		if err != nil {
			log.Error().
				Err(err).
				Str("unit", unitName).
				Msg("Could not change systemd unit enabled state.")
		}
	})
	box.PackEnd(unitEnabledBtn, false, false, 0)

	// a switch to start/stop the mountpoint
	mountToggle, _ := gtk.SwitchNew()
	active, err := systemd.UnitIsActive(unitName)
	if err == nil {
		mountToggle.SetActive(active)
	} else {
		log.Error().Err(err).Msg("Error checking unit active state.")
	}
	mountToggle.SetTooltipText("Mount or unmount selected OneDrive account")
	mountToggle.SetVAlign(gtk.ALIGN_CENTER)
	mountToggle.Connect("state-set", func() {
		err := systemd.UnitSetActive(unitName, mountToggle.GetActive())
		if err != nil {
			log.Error().
				Err(err).
				Str("unit", unitName).
				Msg("Could not change systemd unit active state.")
		}
	})
	box.PackEnd(mountToggle, false, false, 0)

	return row
}
