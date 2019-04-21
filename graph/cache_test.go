// these tests are independent of the mounted fs
package graph

import (
	"fmt"
	"log"
	"testing"
)

func TestRootGet(t *testing.T) {
	cache := NewCache(auth)
	root, err := cache.Get("/", auth)
	if err != nil {
		t.Fatal(err)
	}

	if root.Path() != "/" {
		t.Fatal("Root path did not resolve correctly")
	}
}

func TestRootChildrenUpdate(t *testing.T) {
	cache := NewCache(auth)
	root, _ := cache.Get("/", auth)
	_, err := root.GetChildren(auth)
	if err != nil {
		t.Fatal(err)
	}

	root.mutex.RLock()
	defer root.mutex.RUnlock()
	if _, exists := root.children["documents"]; !exists {
		t.Fatal("Could not find documents folder.")
	}
}

func TestSubdirGet(t *testing.T) {
	cache := NewCache(auth)
	documents, err := cache.Get("/Documents", auth)
	if err != nil {
		t.Fatal(err)
	}
	if documents.Name() != "Documents" {
		t.Fatalf("Failed to fetch \"/Documents\". Got \"%s\" instead!\n", documents.Name())
	}
}

func TestSubdirChildrenUpdate(t *testing.T) {
	cache := NewCache(auth)
	documents, err := cache.Get("/Documents", auth)
	failOnErr(t, err)

	children, _ := documents.GetChildren(auth)
	documents.mutex.RLock()
	defer documents.mutex.RUnlock()
	if _, exists := children["Documents"]; exists {
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
	item, _ := cache.Get("/Documents", auth)
	item2, _ := cache.Get("/Documents", auth)
	if item != item2 {
		t.Fatalf("Pointers to cached items do not match: %p != %p\n", item, item2)
	}
	if item == nil {
		t.Fatal("Item was nil!")
	}
}
