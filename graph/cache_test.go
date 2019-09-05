// these tests are independent of the mounted fs
package graph

import (
	"fmt"
	"log"
	"testing"
)

func TestRootGet(t *testing.T) {
	cache := NewCache(auth)
	root, err := cache.GetPath("/", auth)
	if err != nil {
		t.Fatal(err)
	}

	if root.Path() != "/" {
		t.Fatal("Root path did not resolve correctly")
	}
}

func TestRootChildrenUpdate(t *testing.T) {
	cache := NewCache(auth)
	children, err := cache.GetChildrenPath("/", auth)
	if err != nil {
		t.Fatal(err)
	}

	if _, exists := children["documents"]; !exists {
		t.Fatal("Could not find documents folder.")
	}
}

func TestSubdirGet(t *testing.T) {
	cache := NewCache(auth)
	documents, err := cache.GetPath("/Documents", auth)
	if err != nil {
		t.Fatal(err)
	}
	if documents.Name() != "Documents" {
		t.Fatalf("Failed to fetch \"/Documents\". Got \"%s\" instead!\n", documents.Name())
	}
}

func TestSubdirChildrenUpdate(t *testing.T) {
	cache := NewCache(auth)
	children, err := cache.GetChildrenPath("/Documents", auth)
	failOnErr(t, err)

	if _, exists := children["documents"]; exists {
		log.Println("Documents directory found inside itself. " +
			"Likely the cache did not traverse correctly.\n\nChildren:\n")
		for key := range children {
			fmt.Println(key)
		}
		t.FailNow()
	}
}

func TestSamePointer(t *testing.T) {
	cache := NewCache(auth)
	item, _ := cache.GetPath("/Documents", auth)
	item2, _ := cache.GetPath("/Documents", auth)
	if item != item2 {
		t.Fatalf("Pointers to cached items do not match: %p != %p\n", item, item2)
	}
	if item == nil {
		t.Fatal("Item was nil!")
	}
}

//TODO test setting a parent multiple times

//TODO test removing a parent multiple times
