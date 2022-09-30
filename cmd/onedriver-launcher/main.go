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
	flag "github.com/spf13/pflag"
)

func usage() {
	fmt.Printf(`onedriver-launcher - Manage and configure onedriver mountpoints

Usage: onedriver-launcher [options]

Valid options:
`)
	flag.PrintDefaults()
}

func main() {
	logLevel := flag.StringP("log", "l", "",
		"Set logging level/verbosity for the filesystem. "+
			"Can be one of: fatal, error, warn, info, debug, trace")
	cacheDir := flag.StringP("cache-dir", "c", "",
		"Change the default cache directory used by onedriver. "+
			"Will be created if it does not already exist.")
	configPath := flag.StringP("config-file", "f", common.DefaultConfigPath(),
		"A YAML-formatted configuration file used by onedriver.")
	versionFlag := flag.BoolP("version", "v", false, "Display program version.")
	help := flag.BoolP("help", "h", false, "Displays this help message.")
	flag.Usage = usage
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}
	if *versionFlag {
		fmt.Println("onedriver-launcher", common.Version())
		os.Exit(0)
	}

	// command line options override config options
	config := common.LoadConfig(*configPath)
	if *cacheDir != "" {
		config.CacheDir = *cacheDir
	}
	if *logLevel != "" {
		config.LogLevel = *logLevel
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})
	zerolog.SetGlobalLevel(common.StringToLevel(config.LogLevel))

	log.Info().Msgf("onedriver-launcher %s", common.Version())

	app, err := gtk.ApplicationNew("com.github.jstaf.onedriver", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create application.")
	}
	app.Connect("activate", func(application *gtk.Application) {
		activateCallback(*config, application)
	})
	os.Exit(app.Run(nil))
}

// activateCallback is what actually sets up the application
func activateCallback(config common.Config, app *gtk.Application) {
	window, _ := gtk.ApplicationWindowNew(app)
	window.SetDefaultSize(550, 400)

	header, _ := gtk.HeaderBarNew()
	header.SetShowCloseButton(true)
	header.SetTitle("onedriver")
	window.SetTitlebar(header)

	listbox, _ := gtk.ListBoxNew()
	window.Add(listbox)

	switches := make(map[string]*gtk.Switch)

	mountpointBtn, _ := gtk.ButtonNewFromIconName("list-add-symbolic", gtk.ICON_SIZE_BUTTON)
	mountpointBtn.SetTooltipText("Add a new OneDrive account.")
	mountpointBtn.Connect("clicked", func(button *gtk.Button) {
		mount := ui.DirChooser("Select a mountpoint")
		if !ui.MountpointIsValid(mount) {
			log.Error().Str("mountpoint", mount).
				Msg("Mountpoint was not valid (or user cancelled the operation). " +
					"Mountpoint must be an empty directory.")
			if mount != "" {
				showDialog(
					"Mountpoint was not valid, mountpoint must be an empty directory "+
						"(there might be hidden files).", gtk.MESSAGE_ERROR, window)
			}
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

		row, sw := newMountRow(config, mount)
		switches[mount] = sw
		listbox.Insert(row, -1)

		go xdgOpenDir(mount)
	})
	header.PackStart(mountpointBtn)

	// create a menubutton and assign a popover menu
	menuBtn, _ := gtk.MenuButtonNew()
	icon, _ := gtk.ImageNewFromIconName("open-menu-symbolic", gtk.ICON_SIZE_BUTTON)
	menuBtn.SetImage(icon)
	popover, _ := gtk.PopoverNew(menuBtn)
	menuBtn.SetPopover(popover)
	popover.SetBorderWidth(8)

	// add buttons to menu
	popoverBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
	settings, _ := gtk.ModelButtonNew()
	settings.SetLabel("Settings")
	settings.Connect("clicked", func(button *gtk.ModelButton) {
		log.Info().Msg("clicked settings")
	})
	popoverBox.PackStart(settings, false, true, 0)

	// print version and link to repo

	about, _ := gtk.ModelButtonNew()
	about.SetLabel("About")
	about.Connect("clicked", func(button *gtk.ModelButton) {
		aboutDialog, _ := gtk.AboutDialogNew()
		aboutDialog.SetAuthors([]string{"Jeff Stafford", "https://github.com/jstaf"})
		aboutDialog.SetWebsite("https://github.com/jstaf/onedriver")
		aboutDialog.SetWebsiteLabel("github.com/jstaf/onedriver")
		aboutDialog.SetVersion(fmt.Sprintf("onedriver %s", common.Version()))
		aboutDialog.SetLicenseType(gtk.LICENSE_GPL_3_0)
		logo, err := gtk.ImageNewFromFile("/usr/share/icons/onedriver/onedriver-128.png")
		if err != nil {
			log.Error().Err(err).Msg("Could not find logo.")
		} else {
			aboutDialog.SetLogo(logo.GetPixbuf())
		}
		aboutDialog.Run()
	})
	popoverBox.PackStart(about, false, true, 0)

	popoverBox.ShowAll()
	popover.Add(popoverBox)
	popover.SetPosition(gtk.POS_BOTTOM)
	header.PackEnd(menuBtn)

	mounts := ui.GetKnownMounts(config.CacheDir)
	for _, mount := range mounts {
		mount = unit.UnitNamePathUnescape(mount)

		log.Info().Str("mount", mount).Msg("Found existing mount.")

		row, sw := newMountRow(config, mount)
		switches[mount] = sw
		listbox.Insert(row, -1)
	}

	listbox.Connect("row-activated", func() {
		row := listbox.GetSelectedRow()
		mount, _ := row.GetName()
		unitName := systemd.TemplateUnit(systemd.OnedriverServiceTemplate,
			unit.UnitNamePathEscape(mount))

		log.Debug().
			Str("mount", mount).
			Str("unit", unitName).
			Str("signal", "row-activated").
			Msg("")

		active, _ := systemd.UnitIsActive(unitName)
		if !active {
			err := systemd.UnitSetActive(unitName, true)
			if err != nil {
				log.Error().
					Err(err).
					Str("unit", unitName).
					Msg("Could not set unit state to active.")
			}

		}
		switches[mount].SetActive(true)

		go xdgOpenDir(mount)
	})

	window.ShowAll()
}

func showDialog(msg string, messageType gtk.MessageType, parentWindow gtk.IWindow) {
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

// xdgOpenDir opens a folder in the user's default file browser.
// Should be invoked as a goroutine to not block the main app.
func xdgOpenDir(mount string) {
	log.Debug().Str("dir", mount).Msg("Opening directory.")
	if mount == "" || !ui.PollUntilAvail(mount, -1) {
		log.Error().
			Str("dir", mount).
			Msg("Either directory was invalid or exceeded timeout waiting for fs to become available.")
		return
	}
	cURI := C.CString("file://" + mount)
	C.g_app_info_launch_default_for_uri(cURI, nil, nil)
	C.free(unsafe.Pointer(cURI))
}

// newMountRow constructs a new ListBoxRow with the controls for an individual mountpoint.
func newMountRow(config common.Config, mount string) (*gtk.ListBoxRow, *gtk.Switch) {
	row, _ := gtk.ListBoxRowNew()
	row.SetSelectable(true)
	box, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	row.Add(box)

	escapedMount := unit.UnitNamePathEscape(mount)
	unitName := systemd.TemplateUnit(systemd.OnedriverServiceTemplate, escapedMount)

	var label *gtk.Label
	tildePath := ui.EscapeHome(mount)
	accountName, err := ui.GetAccountName(config.CacheDir, escapedMount)
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
		log.Trace().
			Str("signal", "clicked").
			Str("mount", mount).
			Str("unitName", unitName).
			Msg("Request to delete mount.")

		dialog, _ := gtk.DialogNewWithButtons("Remove mountpoint?", nil, gtk.DIALOG_MODAL,
			[]interface{}{"Cancel", gtk.RESPONSE_REJECT},
			[]interface{}{"Remove", gtk.RESPONSE_ACCEPT},
		)
		if dialog.Run() == gtk.RESPONSE_ACCEPT {
			log.Info().
				Str("signal", "clicked").
				Str("mount", mount).
				Str("unitName", unitName).
				Msg("Deleting mount.")
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
		log.Info().
			Str("signal", "toggled").
			Str("mount", mount).
			Str("unitName", unitName).
			Bool("enabled", unitEnabledBtn.GetActive()).
			Msg("Changing systemd unit enabled state.")
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
		log.Info().
			Str("signal", "state-set").
			Str("mount", mount).
			Str("unitName", unitName).
			Bool("active", mountToggle.GetActive()).
			Msg("Changing systemd unit active state.")
		err := systemd.UnitSetActive(unitName, mountToggle.GetActive())
		if err != nil {
			log.Error().
				Err(err).
				Str("unit", unitName).
				Msg("Could not change systemd unit active state.")
		}
	})
	box.PackEnd(mountToggle, false, false, 0)

	// name is used by "row-activated" callback
	row.SetName(mount)
	row.ShowAll()
	return row, mountToggle
}
