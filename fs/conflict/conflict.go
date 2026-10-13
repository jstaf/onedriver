// Package conflict contains the pure decision logic used when a server-side
// change to a file collides with local, not-yet-uploaded changes. It is kept
// separate from package fs so that it can be unit tested without a live
// OneDrive account.
package conflict

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Action is what should be done with a remote file delta.
type Action int

const (
	// Skip means local state is kept as-is: either the content is identical,
	// or the remote state is something we produced ourselves.
	Skip Action = iota
	// Overwrite means the local copy has nothing worth preserving and should
	// be replaced by the remote version.
	Overwrite
	// Conflict means both sides changed: keep the local version and preserve
	// the remote version as a conflict copy.
	Conflict
	// Unknown means local changes exist but the remote content cannot be
	// compared (no hash available). Local is kept, nothing is overwritten.
	Unknown
)

func (a Action) String() string {
	switch a {
	case Skip:
		return "skip"
	case Overwrite:
		return "overwrite"
	case Conflict:
		return "conflict"
	case Unknown:
		return "unknown"
	}
	return fmt.Sprintf("Action(%d)", int(a))
}

// hashEqual compares two QuickXorHashes the same way graph.VerifyChecksum does.
func hashEqual(a, b string) bool {
	return a != "" && strings.EqualFold(a, b)
}

// Resolve decides what to do with a remote file change.
//   - remoteHash: content hash reported by the server for the new version
//   - localHash: content hash the local inode believes it has
//   - localChanges: true if the local inode has un-uploaded modifications or
//     an upload for it is still queued/in progress
//   - ownHashes: hashes of content this client has uploaded for the item; a
//     remote version matching one of these is an echo of our own upload
func Resolve(remoteHash, localHash string, localChanges bool, ownHashes []string) Action {
	if hashEqual(remoteHash, localHash) {
		return Skip
	}
	if !localChanges {
		return Overwrite
	}
	if remoteHash == "" {
		return Unknown
	}
	for _, own := range ownHashes {
		if hashEqual(remoteHash, own) {
			return Skip
		}
	}
	return Conflict
}

// characters that are not allowed in OneDrive item names
const invalidChars = `"*:<>?/\|`

// sanitize replaces characters that OneDrive refuses in item names.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(invalidChars, r) || r < 0x20 {
			return '-'
		}
		return r
	}, s)
}

// Hostname returns a name for this machine that is safe to embed in a
// OneDrive item name.
func Hostname() string {
	host, err := os.Hostname()
	host = strings.TrimSpace(sanitize(host))
	if err != nil || host == "" {
		return "onedriver"
	}
	return host
}

// Name generates a OneDrive-style conflict copy name, for example
// "report (conflict myhost 2024-01-31 135901).docx". If n > 1 a counter is
// appended to disambiguate repeated conflicts within the same second.
func Name(name, host string, t time.Time, n int) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" || strings.TrimLeft(base, ".") == "" {
		// dotfiles like ".bashrc" have no extension, the whole thing is the name
		base, ext = name, ""
	}
	suffix := fmt.Sprintf("conflict %s %s", sanitize(host), t.Format("2006-01-02 150405"))
	if n > 1 {
		suffix = fmt.Sprintf("%s %d", suffix, n)
	}
	return fmt.Sprintf("%s (%s)%s", base, suffix, ext)
}
