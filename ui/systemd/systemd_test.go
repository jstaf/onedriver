package systemd

import (
	"os"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/unit"
)

// Does systemd unit name templating work correctly?
func TestTemplateUnit(t *testing.T) {
	t.Parallel()
	escaped := TemplateUnit(OnedriverServiceTemplate, "this-is-a-test")
	const expected = "onedriver@this-is-a-test.service"
	if escaped != expected {
		t.Fatalf("Templating did not work. Got \"%s\", wanted \"%s\"\n", escaped, expected)
	}
}

// Does systemd unit untemplating work?
func TestUntemplateUnit(t *testing.T) {
	t.Parallel()
	_, err := UntemplateUnit("this-wont-work")
	if err == nil {
		t.Error("Untemplating \"this-wont-work\" shouldn't have worked.")
	}

	expected := "home-some-path"
	unescaped, err := UntemplateUnit("onedriver@home-some-path")
	if err != nil {
		t.Error("Failed to untemplate unit:", err)
	}
	if unescaped != expected {
		t.Errorf("Did not get expected result. Got: \"%s\", wanted \"%s\"\n", unescaped, expected)
	}

	expected = "opt-other"
	unescaped, err = UntemplateUnit("onedriver@opt-other.service")
	if err != nil {
		t.Error("Failed to untemplate unit:", err)
	}
	if unescaped != expected {
		t.Errorf("Did not get expected result. Got: \"%s\", wanted \"%s\"\n", unescaped, expected)
	}
}

// can we enable and disable systemd units? (and correctly check if the units are
// enabled/disabled?)
func TestUnitEnabled(t *testing.T) {
	t.Parallel()
	testDir, _ := os.Getwd()
	unitName := TemplateUnit(OnedriverServiceTemplate, unit.UnitNamePathEscape(testDir+"/mount"))

	// make sure everything is disabled before we start
	failOnErr(t, UnitSetEnabled(unitName, false))
	enabled, err := UnitIsEnabled(unitName)
	failOnErr(t, err)
	if enabled {
		t.Fatal("Unit was enabled before test started and we couldn't disable it!")
	}

	// actual test content
	failOnErr(t, UnitSetEnabled(unitName, true))
	enabled, err = UnitIsEnabled(unitName)
	failOnErr(t, err)
	if !enabled {
		t.Error("Could not detect unit as enabled.")
	}

	failOnErr(t, UnitSetEnabled(unitName, true))
	enabled, err = UnitIsEnabled(unitName)
	failOnErr(t, err)
	if !enabled {
		t.Error("Unit was still enabled after disabling it.")
	}
}

func TestUnitActive(t *testing.T) {
	t.Parallel()
	testDir, _ := os.Getwd()
	unitName := TemplateUnit(OnedriverServiceTemplate, unit.UnitNamePathEscape(testDir+"/mount"))

	// make extra sure things are off before we start
	failOnErr(t, UnitSetActive(unitName, false))
	active, err := UnitIsActive(unitName)
	failOnErr(t, err)
	if active {
		t.Fatal("Unit was active before job start and we could not stop it!")
	}

	failOnErr(t, UnitSetActive(unitName, true))
	time.Sleep(2 * time.Second)
	active, err = UnitIsActive(unitName)
	failOnErr(t, err)
	if !active {
		t.Error("Could not detect unit as active following start.")
	}

	failOnErr(t, UnitSetActive(unitName, false))
	active, err = UnitIsActive(unitName)
	failOnErr(t, err)
	if active {
		t.Error("Did not detect unit as stopped.")
	}
}
