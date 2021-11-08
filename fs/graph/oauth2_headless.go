// +build !linux !cgo

package graph

// accountName arg is only present for compatibility with the non-headless C version.
func getAuthCode(accountName string) string {
	return getAuthCodeHeadless(accountName)
}
