// +build !linux !cgo

package graph

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// accountName arg is only present for compatibility with the non-headless C version.
func getAuthCode(accountName string) string {
	fmt.Printf("Please visit the following URL:\n%s\n\n", getAuthURL())
	fmt.Println("Please enter the redirect URL once you are redirected to a " +
		"blank page (after \"Let this app access your info?\"):")
	var response string
	fmt.Scanln(&response)
	code, err := parseAuthCode(response)
	if err != nil {
		log.Fatal().Msg("No validation code returned, or code was invalid. " +
			"Please restart the application and try again.")
	}
	return code
}
