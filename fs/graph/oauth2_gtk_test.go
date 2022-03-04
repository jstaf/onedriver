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
