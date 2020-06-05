package graph

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/rclone/rclone/backend/onedrive/quickxorhash"
)

// SHA1Hash returns the SHA1 hash of some data as a string
func SHA1Hash(data *[]byte) string {
	// the onedrive API returns SHA1 hashes in all caps, so we do too
	return strings.ToUpper(fmt.Sprintf("%x", sha1.Sum(*data)))
}

// QuickXORHash computes the Microsoft-specific QuickXORHash. Reusing rclone's
// implementation until I get the chance to rewrite/add test cases to remove the
// dependency.
func QuickXORHash(data *[]byte) string {
	hash := quickxorhash.Sum(*data)
	return base64.StdEncoding.EncodeToString(hash[:])
}

// VerifyChecksum checks to see if a DriveItem's checksum matches what it's
// supposed to be. This is less of a cryptographic check and more of a file
// integrity check.
func (d *DriveItem) VerifyChecksum(checksum string) bool {
	if len(checksum) == 0 {
		return false
	}
	// all checksums are converted to upper to avoid casing issues from whatever
	// the API decides to return at this point in time.
	checksum = strings.ToUpper(checksum)
	return strings.ToUpper(d.File.Hashes.SHA1Hash) == checksum ||
		strings.ToUpper(d.File.Hashes.QuickXorHash) == checksum
}
