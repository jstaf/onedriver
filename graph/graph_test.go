package graph

import (
	"os"
	"testing"
	"time"
)

var auth Auth

func TestMain(m *testing.M) {
	auth = Authenticate()
	os.Exit(m.Run())
}

func TestRequestUnauthenticated(t *testing.T) {
	badAuth := Auth{
		// Set a renewal 1 year in the future so we don't accidentally overwrite
		// our auth tokens
		ExpiresAt: time.Now().Unix() + 60*60*24*365,
	}
	_, err := Get("/me/drive/root", badAuth)
	if err == nil {
		t.Fatal("An unauthenticated request was not handled as an error")
	}
}

func TestGetItem(t *testing.T) {
	item, err := GetItem("/", auth)
	if item.Name != "root" {
		t.Fatal("Failed to fetch directory root. Addtional errors:", err)
	}

	item, err = GetItem("/lkjfsdlfjdwjkfl", auth)
	if err == nil {
		t.Fatal("We didn't return an error for a non-existent item!")
	}
}

func TestGetChildren(t *testing.T) {
	reqCache := NewRequestCache()
	items, err := GetChildren("/", auth, reqCache)
	var success bool
	for _, item := range items {
		if item.Name == "Documents" {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("Could not find the '/Documents' folder as a child of '/'!")
	}

	items, err = GetChildren("/lkdsjflkdjsfl", auth, reqCache)
	if err == nil {
		t.Fatal("GetChildren() for a non-existent directory did not throw an error")
	}
}
