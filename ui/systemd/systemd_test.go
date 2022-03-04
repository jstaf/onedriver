package systemd

import (
	"os"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/unit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Does systemd unit name templating work correctly?
func TestTemplateUnit(t *testing.T) {
	t.Parallel()
	escaped := TemplateUnit(OnedriverServiceTemplate, "this-is-a-test")
	require.Equal(t, "onedriver@this-is-a-test.service", escaped, "Templating did not work.")
}

// Does systemd unit untemplating work?
func TestUntemplateUnit(t *testing.T) {
	t.Parallel()
	_, err := UntemplateUnit("this-wont-work")
	assert.Error(t, err, "Untemplating \"this-wont-work\" shouldn't have worked.")

	unescaped, err := UntemplateUnit("onedriver@home-some-path")
	assert.NoError(t, err, "Failed to untemplate unit.")
	assert.Equal(t, "home-some-path", unescaped, "Did not untemplate systemd unit correctly.")

	unescaped, err = UntemplateUnit("onedriver@opt-other.service")
	assert.NoError(t, err, "Failed to untemplate unit.")
	assert.Equal(t, "opt-other", unescaped, "Did not untemplate systemd unit correctly.")
}

// can we enable and disable systemd units? (and correctly check if the units are
// enabled/disabled?)
func TestUnitEnabled(t *testing.T) {
	t.Parallel()
	testDir, _ := os.Getwd()
	unitName := TemplateUnit(OnedriverServiceTemplate, unit.UnitNamePathEscape(testDir+"/mount"))

	// make sure everything is disabled before we start
	require.NoError(t, UnitSetEnabled(unitName, false))
	enabled, err := UnitIsEnabled(unitName)
	require.NoError(t, err)
	require.False(t, enabled, "Unit was enabled before test started and we couldn't disable it!")

	// actual test content
	require.NoError(t, UnitSetEnabled(unitName, true))
	enabled, err = UnitIsEnabled(unitName)
	require.NoError(t, err)
	require.True(t, enabled, "Could not detect unit as enabled.")

	require.NoError(t, UnitSetEnabled(unitName, false))
	enabled, err = UnitIsEnabled(unitName)
	require.NoError(t, err)
	require.False(t, enabled, "Unit was still enabled after disabling it.")
}

func TestUnitActive(t *testing.T) {
	t.Parallel()
	testDir, _ := os.Getwd()
	unitName := TemplateUnit(OnedriverServiceTemplate, unit.UnitNamePathEscape(testDir+"/mount"))

	// make extra sure things are off before we start
	require.NoError(t, UnitSetActive(unitName, false))
	active, err := UnitIsActive(unitName)
	require.NoError(t, err)
	require.False(t, active, "Unit was active before job start and we could not stop it!")

	require.NoError(t, UnitSetActive(unitName, true), "Failed to start unit.")
	time.Sleep(2 * time.Second)
	active, err = UnitIsActive(unitName)
	require.NoError(t, err, "Failed to check unit active state.")
	require.True(t, active, "Could not detect unit as active following start.")

	require.NoError(t, UnitSetActive(unitName, false), "Failed to stop unit.")
	active, err = UnitIsActive(unitName)
	require.NoError(t, err, "Failed to check unit active state.")
	require.False(t, active, "Did not detect unit as stopped.")
}
