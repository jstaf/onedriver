package ui

import "testing"

// Does systemd unit name templating work correctly?
func TestSystemdTemplateUnit(t *testing.T) {
	escaped := SystemdTemplateUnit(OnedriverServiceTemplate, "this-is-a-test")
	const expected = "onedriver@this-is-a-test.service"
	if escaped != expected {
		t.Fatalf("Templating did not work. Got \"%s\", wanted \"%s\"\n", escaped, expected)
	}
}

// Does systemd unit untemplating work?
func TestSystemdUntemplateUnit(t *testing.T) {
	_, err := SystemdUntemplateUnit("this-wont-work")
	if err == nil {
		t.Error("Untemplating \"this-wont-work\" shouldn't have worked.")
	}

	expected := "home-some-path"
	unescaped, err := SystemdUntemplateUnit("onedriver@home-some-path")
	if err != nil {
		t.Error("Failed to untemplate unit:", err)
	}
	if unescaped != expected {
		t.Errorf("Did not get expected result. Got: \"%s\", wanted \"%s\"\n", unescaped, expected)
	}

	expected = "opt-other"
	unescaped, err = SystemdUntemplateUnit("onedriver@opt-other.service")
	if err != nil {
		t.Error("Failed to untemplate unit:", err)
	}
	if unescaped != expected {
		t.Errorf("Did not get expected result. Got: \"%s\", wanted \"%s\"\n", unescaped, expected)
	}
}
