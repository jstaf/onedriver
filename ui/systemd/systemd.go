package systemd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
)

const (
	OnedriverServiceTemplate = "onedriver@.service"
	SystemdBusName           = "org.freedesktop.systemd1"
	SystemdObjectPath        = "/org/freedesktop/systemd1"
)

// TemplateUnit templates a unit name as systemd would
func TemplateUnit(template, instance string) string {
	return strings.Replace(template, "@.", fmt.Sprintf("@%s.", instance), 1)
}

// UntemplateUnit reverses the templating done by SystemdTemplateUnit
func UntemplateUnit(unit string) (string, error) {
	var start, end int
	for i, char := range unit {
		if char == '@' {
			start = i + 1
		}
		if char == '.' {
			break
		}
		end = i + 1
	}
	if start == 0 {
		return "", errors.New("not a systemd templated unit")
	}
	return unit[start:end], nil
}

// UnitIsActive returns true if the unit is currently active
func UnitIsActive(unit string) (bool, error) {
	conn, err := dbus.NewUserConnectionContext(context.Background())
	if err != nil {
		return false, err
	}
	defer conn.Close()
	return false, nil
}

func UnitSetActive(unit string, active bool) error {
	ctx := context.Background()
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	status := make(chan string)
	if active {
		_, err = conn.StartUnitContext(context.Background(), unit, "replace", status)
	} else {
		_, err = conn.StopUnitContext(context.Background(), unit, "replace", status)

	}
	if err != nil {
		return err
	}

	if result := <-status; result != "done" {
		return errors.New(fmt.Sprintf("job failed with status %s", result))
	}
	return nil
}

// UnitIsEnabled returns true if a particular systemd unit is enabled.
func UnitIsEnabled(unit string) (bool, error) {
	conn, err := godbus.ConnectSessionBus()
	if err != nil {
		return false, err
	}
	defer conn.Close()

	var state string
	obj := conn.Object(SystemdBusName, SystemdObjectPath)
	err = obj.Call(
		"org.freedesktop.systemd1.Manager.GetUnitFileState", 0, unit,
	).Store(&state)
	if err != nil {
		return false, err
	}
	return state == "enabled", nil
}

// UnitSetEnabled sets a systemd unit to enabled/disabled.
func UnitSetEnabled(unit string, enabled bool) error {
	ctx := context.Background()
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	units := []string{unit}
	if enabled {
		_, _, err = conn.EnableUnitFilesContext(ctx, units, false, true)
	} else {
		_, err = conn.DisableUnitFilesContext(ctx, units, false)
	}
	return err
}
