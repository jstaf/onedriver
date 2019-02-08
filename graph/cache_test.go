package graph

import "testing"

func TestSamePointer(t *testing.T) {
	item, _ := CacheGetItem("/", auth)
	item2, _ := CacheGetItem("/", auth)
	if item != item2 {
		t.Fatalf("Pointers to cached items do not match: %p != %p\n", item, item2)
	}
	if item == nil {
		t.Fatal("Item was nil!")
	}
}
