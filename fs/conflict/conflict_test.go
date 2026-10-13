package conflict

import (
	"strings"
	"testing"
	"time"
)

func TestName(t *testing.T) {
	ts := time.Date(2024, 1, 31, 13, 59, 1, 0, time.UTC)
	tests := []struct {
		name string
		host string
		n    int
		want string
	}{
		{"report.docx", "myhost", 0, "report (conflict myhost 2024-01-31 135901).docx"},
		{"report.docx", "myhost", 1, "report (conflict myhost 2024-01-31 135901).docx"},
		{"report.docx", "myhost", 3, "report (conflict myhost 2024-01-31 135901 3).docx"},
		{"Makefile", "myhost", 0, "Makefile (conflict myhost 2024-01-31 135901)"},
		{".bashrc", "myhost", 0, ".bashrc (conflict myhost 2024-01-31 135901)"},
		{"archive.tar.gz", "myhost", 0, "archive.tar (conflict myhost 2024-01-31 135901).gz"},
		{"notes.txt", `bad/host:name*`, 0, "notes (conflict bad-host-name- 2024-01-31 135901).txt"},
		{"with space.md", "h", 0, "with space (conflict h 2024-01-31 135901).md"},
	}
	for _, tc := range tests {
		if got := Name(tc.name, tc.host, ts, tc.n); got != tc.want {
			t.Errorf("Name(%q, %q, %d) = %q, want %q", tc.name, tc.host, tc.n, got, tc.want)
		}
	}
}

func TestHostnameSafe(t *testing.T) {
	host := Hostname()
	if host == "" {
		t.Fatal("hostname should never be empty")
	}
	if strings.ContainsAny(host, invalidChars) {
		t.Fatalf("hostname %q contains characters invalid for OneDrive", host)
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		desc         string
		remote       string
		local        string
		localChanges bool
		own          []string
		want         Action
	}{
		{"same content, clean", "AAA", "AAA", false, nil, Skip},
		{"same content, case-insensitive", "aaa", "AAA", true, nil, Skip},
		{"remote changed, clean", "BBB", "AAA", false, nil, Overwrite},
		{"remote changed, no remote hash, clean", "", "AAA", false, nil, Overwrite},
		{"no hashes at all, clean", "", "", false, nil, Overwrite},
		{"remote changed, local changed", "BBB", "AAA", true, nil, Conflict},
		{"remote changed, local changed, unrelated own hashes", "BBB", "AAA", true, []string{"CCC"}, Conflict},
		{"remote is echo of our own earlier upload", "BBB", "AAA", true, []string{"CCC", "bbb"}, Skip},
		{"remote hash unknown, local changed", "", "AAA", true, nil, Unknown},
		{"remote hash unknown, local changed, empty own hash", "", "AAA", true, []string{""}, Unknown},
	}
	for _, tc := range tests {
		if got := Resolve(tc.remote, tc.local, tc.localChanges, tc.own); got != tc.want {
			t.Errorf("%s: Resolve() = %s, want %s", tc.desc, got, tc.want)
		}
	}
}
