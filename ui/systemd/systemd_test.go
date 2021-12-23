package systemd

import "testing"

// Does systemd unit name templating work correctly?
func TestTemplateUnit(t *testing.T) {
	escaped := TemplateUnit(OnedriverServiceTemplate, "this-is-a-test")
	const expected = "onedriver@this-is-a-test.service"
	if escaped != expected {
		t.Fatalf("Templating did not work. Got \"%s\", wanted \"%s\"\n", escaped, expected)
	}
}

// Does systemd unit untemplating work?
func TestUntemplateUnit(t *testing.T) {
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
