package graph

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/jstaf/onedriver/fs/graph/quickxorhash"
)

func SHA256Hash(data *[]byte) string {
	return strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256(*data)))
}

func SHA256HashStream(reader io.ReadSeeker) string {
	reader.Seek(0, 0)
	hash := sha256.New()
	io.Copy(hash, reader)
	reader.Seek(0, 0)
	return strings.ToUpper(fmt.Sprintf("%x", hash.Sum(nil)))
}

// SHA1Hash returns the SHA1 hash of some data as a string
func SHA1Hash(data *[]byte) string {
	// the onedrive API returns SHA1 hashes in all caps, so we do too
	return strings.ToUpper(fmt.Sprintf("%x", sha1.Sum(*data)))
}

// SHA1HashStream hashes the contents of a stream.
func SHA1HashStream(reader io.ReadSeeker) string {
	reader.Seek(0, 0)
	hash := sha1.New()
	io.Copy(hash, reader)
	reader.Seek(0, 0)
	return strings.ToUpper(fmt.Sprintf("%x", hash.Sum(nil)))
}

// QuickXORHash computes the Microsoft-specific QuickXORHash. Reusing rclone's
// implementation until I get the chance to rewrite/add test cases to remove the
// dependency.
func QuickXORHash(data *[]byte) string {
	hash := quickxorhash.Sum(*data)
	return base64.StdEncoding.EncodeToString(hash[:])
}

// QuickXORHashStream hashes a stream.
func QuickXORHashStream(reader io.ReadSeeker) string {
	reader.Seek(0, 0)
	hash := quickxorhash.New()
	io.Copy(hash, reader)
	reader.Seek(0, 0)
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

// VerifyChecksum checks to see if a DriveItem's checksum matches what it's
// supposed to be. This is less of a cryptographic check and more of a file
// integrity check.
func (d *DriveItem) VerifyChecksum(checksum string) bool {
	if len(checksum) == 0 || d.File == nil {
		return false
	}
	return strings.EqualFold(d.File.Hashes.QuickXorHash, checksum)
}

// ETagIsMatch returns true if the etag matches the one in the DriveItem
func (d *DriveItem) ETagIsMatch(etag string) bool {
	return d.ETag != "" && d.ETag == etag
}
