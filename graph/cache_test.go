package graph

import "testing"

func TestSamePointer(t *testing.T) {
	cache := NewItemCache()
	item, _ := cache.Get("/", auth)
	item2, _ := cache.Get("/", auth)
	if item != item2 {
		t.Fatalf("Pointers to cached items do not match: %p != %p\n", item, item2)
	}
	if item == nil {
		t.Fatal("Item was nil!")
	}
}
