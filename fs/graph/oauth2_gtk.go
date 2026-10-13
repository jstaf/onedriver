//go:build linux && cgo
// +build linux,cgo

package graph

/*
#cgo linux pkg-config: webkit2gtk-4.1
#include "stdlib.h"
#include "oauth2_gtk.h"
*/
import "C"

import (
	"os/exec"
	"unsafe"

	"github.com/rs/zerolog/log"
)

// Fetch the auth code required as the first part of oauth2 authentication. Uses
// webkit2gtk to create a popup browser.
func getAuthCode(a AuthConfig, accountName string) string {
	warnMissingGstreamerPlugins()
	cAuthURL := C.CString(getAuthURL(a))
	cAccountName := C.CString(accountName)
	cResponse := C.webkit_auth_window(cAuthURL, cAccountName)
	response := C.GoString(cResponse)
	C.free(unsafe.Pointer(cAuthURL))
	C.free(unsafe.Pointer(cAccountName))
	C.free(unsafe.Pointer(cResponse))

	code, err := parseAuthCode(response)
	if err != nil || code == "" {
		//TODO create a popup with the auth failure message here instead of a log message
		log.Fatal().Msg("Login did not complete: no validation code returned, or code " +
			"was invalid (was the login window closed, or did the login page not work " +
			"correctly?). Please restart the application and try again. If the login " +
			"window keeps failing, run \"onedriver -n\" to log in using your normal " +
			"web browser instead.")
	}
	return code
}

// warnMissingGstreamerPlugins warns if GStreamer's autoaudiosink element is
// missing (it is part of gst-plugins-good). Without it, WebKitGTK's web process
// breaks while loading the Microsoft login page, and the number to enter in the
// Microsoft Authenticator app is never shown.
func warnMissingGstreamerPlugins() {
	inspect, err := exec.LookPath("gst-inspect-1.0")
	if err != nil {
		return // can't tell, so don't guess
	}
	if exec.Command(inspect, "--exists", "autoaudiosink").Run() != nil {
		log.Warn().Msg("The GStreamer element autoaudiosink was not found. The login " +
			"window may not work correctly (for instance, it may not show the " +
			"Microsoft Authenticator number). Install gst-plugins-good " +
			"(gstreamer1.0-plugins-good on Debian/Ubuntu), or log in with " +
			"\"onedriver -n\" instead.")
	}
}

// uriGetHost is exclusively here for testing because we cannot use CGo in tests,
// but can use functions that invoke CGo in tests.
func uriGetHost(uri string) string {
	input := C.CString(uri)
	defer C.free(unsafe.Pointer(input))

	host := C.uri_get_host(input)
	defer C.free(unsafe.Pointer(host))
	if host == nil {
		return ""
	}
	return C.GoString(host)
}
