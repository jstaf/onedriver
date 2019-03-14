package logger

import (
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
)

func TestNormalPrint(t *testing.T) {
	temp, _ := ioutil.TempFile("", "logger_test_*")
	log.SetOutput(temp)
	defer os.Remove(temp.Name())
	defer temp.Close()

	text := "This is a test"
	Info(text)

	read, _ := ioutil.ReadFile(temp.Name())
	contents := string(read)
	if !strings.Contains(contents, text) {
		t.Fatalf("Did not contain expected output.\nGot: \"%s\"\nWanted: \"%s\"\n",
			contents, text)
	}

	if strings.Contains(contents, "["+text+"]") {
		t.Fatalf("Did not contain expected output.\nGot: \"%s\"\nWanted: \"%s\"\n",
			contents, text)
	}
}

func TestMultiplePrint(t *testing.T) {
	temp, _ := ioutil.TempFile("", "logger_test_*")
	log.SetOutput(temp)
	defer os.Remove(temp.Name())
	defer temp.Close()

	Info("separate", "words", "this", "time", 42)

	read, _ := ioutil.ReadFile(temp.Name())
	contents := string(read)
	toMatch := "separate words this time 42"
	if !strings.Contains(contents, toMatch) {
		t.Fatalf("Did not contain expected output.\nGot: \"%s\"\nWanted: \"%s\"\n",
			contents, toMatch)
	}
}
func TestPrintf(t *testing.T) {
	temp, _ := ioutil.TempFile("", "logger_test_*")
	log.SetOutput(temp)
	defer os.Remove(temp.Name())
	defer temp.Close()

	Infof("%d %s\n", 26, "is nice")

	read, _ := ioutil.ReadFile(temp.Name())
	contents := string(read)
	toMatch := "26 is nice"
	if !strings.Contains(contents, toMatch) {
		t.Fatalf("Did not contain expected output.\nGot: \"%s\"\nWanted: \"%s\"\n",
			contents, toMatch)
	}
}
