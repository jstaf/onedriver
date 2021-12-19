package ui

import (
	"errors"
	"fmt"
	"strings"
)

// SystemdTemplateUnit templates a unit name as systemd would
func SystemdTemplateUnit(template, instance string) string {
	return strings.Replace(template, "@.", fmt.Sprintf("@%s.", instance), 1)
}

// SystemdUntemplateUnit reverses the templating done by SystemdTemplateUnit
func SystemdUntemplateUnit(unit string) (string, error) {
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
