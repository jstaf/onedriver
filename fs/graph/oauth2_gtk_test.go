//go:build linux && cgo
// +build linux,cgo

package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURIGetHost(t *testing.T) {
	host := uriGetHost("this won't work")
	assert.Equal(t, "", host, "Func should return NULL if not a valid URI")

	host = uriGetHost("https://account.live.com/test/index.html")
	assert.Equal(t, "account.live.com", host, "Failed URI host with extra path.")

	host = uriGetHost("http://account.live.com")
	assert.Equal(t, "account.live.com", host, "Failed URI host without extra path")
}

func TestURIGetHostEdgeCases(t *testing.T) {
	assert.Equal(t, "", uriGetHost(""))
	assert.Equal(t, "", uriGetHost("/"))
	assert.Equal(t, "a.b", uriGetHost("https://a.b/"))
	assert.Equal(t, "login.live.com", uriGetHost("https://login.live.com/oauth20_desktop.srf?code=x"))
}
