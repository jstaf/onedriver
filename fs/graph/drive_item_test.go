package graph

import "testing"

func TestGetItem(t *testing.T) {
	t.Parallel()
	var auth Auth
	auth.FromFile(".auth_tokens.json")
	item, err := GetItemPath("/", &auth)
	if item.Name != "root" {
		t.Fatal("Failed to fetch directory root. Additional errors:", err)
	}

	_, err = GetItemPath("/lkjfsdlfjdwjkfl", &auth)
	if err == nil {
		t.Fatal("We didn't return an error for a non-existent item!")
	}
}
