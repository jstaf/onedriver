package fs

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
