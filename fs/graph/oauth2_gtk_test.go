// +build linux,cgo

package graph

import "testing"

func TestURIGetHost(t *testing.T) {
	host := uriGetHost("this won't work")
	if host != "" {
		t.Errorf("Func should return NULL if not a valid URI, got %s\n", host)
	}

	host = uriGetHost("https://account.live.com/test/index.html")
	if host != "account.live.com" {
		t.Errorf("Got %s, wanted \"account.live.com\"\n", host)
	}

	host = uriGetHost("http://account.live.com")
	if host != "account.live.com" {
		t.Errorf("Got %s, wanted \"account.live.com\"\n", host)
	}
}
