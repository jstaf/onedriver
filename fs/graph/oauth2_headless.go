// +build !linux !cgo

package graph

import (
	"fmt"
	"regexp"

	log "github.com/sirupsen/logrus"
)

func getAuthCode() string {
	fmt.Printf("Please visit the following URL:\n%s\n\n", getAuthURL())
	fmt.Println("Please enter the redirect URL once you are redirected to a " +
		"blank page (after \"Let this app access your info?\"):")
	var response string
	fmt.Scanln(&response)

	rexp := regexp.MustCompile("code=([a-zA-Z0-9-_])+")
	code := rexp.FindString(response)
	if len(code) == 0 {
		log.Fatal("No validation code returned, or code was invalid. " +
			"Please restart the application and try again.")
	}
	return code[5:]
}
