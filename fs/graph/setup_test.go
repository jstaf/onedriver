package graph

import (
	"os"
	"testing"

	"github.com/jstaf/onedriver/logger"
	log "github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	os.Chdir("../..")
	f := logger.LogTestSetup()
	defer f.Close()

	// auth and log account metadata so we're extra sure who we're testing against
	auth := Authenticate(".auth_tokens.json")
	user, _ := GetUser(auth)
	drive, _ := GetDrive(auth)
	log.WithFields(log.Fields{
		"account": user.UserPrincipalName,
		"type":    drive.DriveType,
	}).Info("Starting tests")

	os.Exit(m.Run())
}
