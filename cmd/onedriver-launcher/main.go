package main

/*
#cgo linux pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
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

	// loading config can emit an unformatted log message, so we do this first
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})

	// command line options override config options
	config := common.LoadConfig(*configPath)
	if *cacheDir != "" {
		config.CacheDir = *cacheDir
	}
	if *logLevel != "" {
		config.LogLevel = *logLevel
	}

	zerolog.SetGlobalLevel(common.StringToLevel(config.LogLevel))

	log.Info().Msgf("onedriver-launcher %s", common.Version())

	app, err := gtk.ApplicationNew("com.github.jstaf.onedriver", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create application.")
	}
	app.Connect("activate", func(application *gtk.Application) {
		activateCallback(application, config, *configPath)
	})
	os.Exit(app.Run(nil))
}

// activateCallback is what actually sets up the application
func activateCallback(app *gtk.Application, config *common.Config, configPath string) {
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
				ui.Dialog(
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

		row, sw := newMountRow(*config, mount)
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
		newSettingsWindow(config, configPath)
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

		row, sw := newMountRow(*config, mount)
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
// mount is the path to the new mountpoint.
func newMountRow(config common.Config, mount string) (*gtk.ListBoxRow, *gtk.Switch) {
	row, _ := gtk.ListBoxRowNew()
	row.SetSelectable(true)
	box, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	row.Add(box)

	escapedMount := unit.UnitNamePathEscape(mount)
	unitName := systemd.TemplateUnit(systemd.OnedriverServiceTemplate, escapedMount)

	driveName, err := common.GetXDGVolumeInfoName(filepath.Join(mount, ".xdg-volume-info"))
	if err != nil {
		log.Error().
			Err(err).
			Str("mountpoint", mount).
			Msg("Could not determine user-specified acccount name.")
	}

	tildePath := ui.EscapeHome(mount)
	accountName, err := ui.GetAccountName(config.CacheDir, escapedMount)
	label, _ := gtk.LabelNew("")
	if driveName != "" {
		// we have a user-assigned name for the user's drive
		label.SetMarkup(fmt.Sprintf("%s <span style=\"italic\" weight=\"light\">(%s)</span>    ",
			driveName, tildePath,
		))
	} else if err == nil {
		// fs isn't mounted, so just use user principal name from AAD
		label, _ = gtk.LabelNew("")
		label.SetMarkup(fmt.Sprintf("%s <span style=\"italic\" weight=\"light\">(%s)</span>    ",
			accountName, tildePath,
		))
	} else {
		// something went wrong and all we have is the mountpoint name
		log.Error().
			Err(err).
			Str("mountpoint", mount).
			Msg("Could not determine user principal name.")
		label, _ = gtk.LabelNew(tildePath)
	}
	box.PackStart(label, false, false, 5)

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

	mountpointSettingsBtn, _ := gtk.MenuButtonNew()
	icon, _ := gtk.ImageNewFromIconName("applications-system-symbolic", gtk.ICON_SIZE_BUTTON)
	mountpointSettingsBtn.SetImage(icon)
	popover, _ := gtk.PopoverNew(mountpointSettingsBtn)
	mountpointSettingsBtn.SetPopover(popover)
	popover.SetBorderWidth(8)
	popoverBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)

	if accountName != "" {
		accountLabel, _ := gtk.LabelNew(accountName)
		popoverBox.Add(accountLabel)
	}
	// rename the mount by rewriting the .xdg-volume-info file
	renameMountpointEntry, _ := gtk.EntryNew()
	renameMountpointEntry.SetTooltipText("Change the label that your file browser uses for this drive")
	renameMountpointEntry.SetText(driveName)
	// runs on enter
	renameMountpointEntry.Connect("activate", func(entry *gtk.Entry) {
		newName, err := entry.GetText()
		ctx := log.With().
			Str("signal", "clicked").
			Str("mount", mount).
			Str("unitName", unitName).
			Str("oldName", driveName).
			Str("newName", newName).
			Logger()
		if err != nil {
			ctx.Error().Err(err).Msg("Failed to get new drive name.")
			return
		}
		if driveName == newName {
			ctx.Info().Msg("New name is same as old name, ignoring.")
			return
		}
		ctx.Info().
			Msg("Renaming mount.")
		popover.GrabFocus()

		err = systemd.UnitSetActive(unitName, true)
		if err != nil {
			ctx.Error().Err(err).Msg("Failed to start mount for rename.")
			return
		}
		mountToggle.SetActive(true)

		if ui.PollUntilAvail(mount, -1) {
			xdgVolumeInfo := common.TemplateXDGVolumeInfo(newName)
			driveName = newName
			//FIXME why does this not work???
			err = ioutil.WriteFile(filepath.Join(mount, ".xdg-volume-info"), []byte(xdgVolumeInfo), 0644)
			if err != nil {
				ctx.Error().Err(err).Msg("Failed to write new mount name.")
				return
			}
		} else {
			ctx.Error().Err(err).Msg("Mount never became ready.")
		}
		// update label in UI now
		label.SetMarkup(fmt.Sprintf("%s <span style=\"italic\" weight=\"light\">(%s)</span>    ",
			newName, tildePath,
		))

		ui.Dialog("Drive rename will take effect on next filesystem start.", gtk.MESSAGE_INFO, nil)
		ctx.Info().Msg("Drive rename will take effect on next filesystem start.")
	})
	popoverBox.Add(renameMountpointEntry)

	separator, _ := gtk.SeparatorMenuItemNew()
	popoverBox.Add(separator)

	// create a button to enable/disable the mountpoint
	unitEnabledBtn, _ := gtk.CheckButtonNewWithLabel(" Start drive on login")
	unitEnabledBtn.SetTooltipText("Start this drive automatically when you login")
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
	popoverBox.PackStart(unitEnabledBtn, false, true, 0)

	// button to delete the mount
	deleteMountpointBtn, _ := gtk.ModelButtonNew()
	deleteMountpointBtn.SetLabel("Remove drive")
	deleteMountpointBtn.SetTooltipText("Remove OneDrive account from local computer")
	deleteMountpointBtn.Connect("clicked", func(button *gtk.ModelButton) {
		log.Trace().
			Str("signal", "clicked").
			Str("mount", mount).
			Str("unitName", unitName).
			Msg("Request to delete drive.")

		if ui.CancelDialog(nil, "<span weight=\"bold\">Remove drive?</span>",
			"This will remove all data for this drive from your local computer. "+
				"It can also be used to \"reset\" the drive to its original state.") {
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
	})
	popoverBox.PackStart(deleteMountpointBtn, false, true, 0)

	// ok show everything in the mount settings menu
	popoverBox.ShowAll()
	popover.Add(popoverBox)
	popover.SetPosition(gtk.POS_BOTTOM)

	// add all widgets to row in the right order
	box.PackEnd(mountpointSettingsBtn, false, false, 0)
	box.PackEnd(mountToggle, false, false, 0)

	// name is used by "row-activated" callback
	row.SetName(mount)
	row.ShowAll()
	return row, mountToggle
}

func newSettingsWindow(config *common.Config, configPath string) {
	const offset = 15

	settingsWindow, _ := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	settingsWindow.SetResizable(false)
	settingsWindow.SetTitle("Settings")

	// log level settings
	settingsRowLog, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, offset)
	logLevelLabel, _ := gtk.LabelNew("Log level")
	settingsRowLog.PackStart(logLevelLabel, false, false, 0)

	logLevelSelector, _ := gtk.ComboBoxTextNew()
	for i, entry := range common.LogLevels() {
		logLevelSelector.AppendText(entry)
		if entry == config.LogLevel {
			logLevelSelector.SetActive(i)
		}
	}
	logLevelSelector.Connect("changed", func(box *gtk.ComboBoxText) {
		config.LogLevel = box.GetActiveText()
		log.Debug().
			Str("newLevel", config.LogLevel).
			Msg("Log level changed.")
		zerolog.SetGlobalLevel(common.StringToLevel(config.LogLevel))
		config.WriteConfig(configPath)
	})
	settingsRowLog.PackEnd(logLevelSelector, false, false, 0)

	// cache dir settings
	settingsRowCacheDir, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, offset)
	cacheDirLabel, _ := gtk.LabelNew("Cache directory")
	settingsRowCacheDir.PackStart(cacheDirLabel, false, false, 0)

	cacheDirPicker, _ := gtk.ButtonNew()
	cacheDirPicker.SetLabel(ui.EscapeHome(config.CacheDir))
	cacheDirPicker.SetSizeRequest(200, 0)
	cacheDirPicker.Connect("clicked", func(button *gtk.Button) {
		oldPath, _ := button.GetLabel()
		oldPath = ui.UnescapeHome(oldPath)
		path := ui.DirChooser("Select an empty directory to use for storage")
		if !ui.CancelDialog(settingsWindow, "Remount all drives?", "") {
			return
		}
		log.Warn().
			Str("oldPath", oldPath).
			Str("newPath", path).
			Msg("All active drives will be remounted to move cache directory.")

		// actually perform the stop+move op
		isMounted := make([]string, 0)
		for _, mount := range ui.GetKnownMounts(oldPath) {
			unitName := systemd.TemplateUnit(systemd.OnedriverServiceTemplate, mount)
			log.Info().
				Str("mount", mount).
				Str("unit", unitName).
				Msg("Disabling mount.")
			if mounted, _ := systemd.UnitIsActive(unitName); mounted {
				isMounted = append(isMounted, unitName)
			}

			err := systemd.UnitSetActive(unitName, false)
			if err != nil {
				ui.Dialog("Could not disable mount: "+err.Error(),
					gtk.MESSAGE_ERROR, settingsWindow)
				log.Error().
					Err(err).
					Str("mount", mount).
					Str("unit", unitName).
					Msg("Could not disable mount.")
				return
			}

			err = os.Rename(filepath.Join(oldPath, mount), filepath.Join(path, mount))
			if err != nil {
				ui.Dialog("Could not move cache for mount: "+err.Error(),
					gtk.MESSAGE_ERROR, settingsWindow)
				log.Error().
					Err(err).
					Str("mount", mount).
					Str("unit", unitName).
					Msg("Could not move cache for mount.")
				return
			}
		}

		// remount drives that were mounted before
		for _, unitName := range isMounted {
			err := systemd.UnitSetActive(unitName, true)
			if err != nil {
				log.Error().
					Err(err).
					Str("unit", unitName).
					Msg("Failed to restart unit.")
			}
		}

		// all done
		config.CacheDir = path
		config.WriteConfig(configPath)
		button.SetLabel(path)
	})
	settingsRowCacheDir.PackEnd(cacheDirPicker, false, false, 0)

	// assemble rows
	settingsWindowBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, offset)
	settingsWindowBox.SetBorderWidth(offset)
	settingsWindowBox.PackStart(settingsRowLog, true, true, 0)
	settingsWindowBox.PackStart(settingsRowCacheDir, true, true, 0)
	settingsWindow.Add(settingsWindowBox)
	settingsWindow.ShowAll()
}
