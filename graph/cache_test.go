package graph

import (
	"fmt"
	"log"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
)

func TestCacheRoot(t *testing.T) {
	cache := ItemCache{}
	root, err := cache.Get("/", auth)
	if err != nil {
		t.Fatal(err)
	}

	if root.Path() != "/" {
		t.Fatal("Root path did not resolve correctly")
	}
}

func TestRootChildrenUpdate(t *testing.T) {
	cache := ItemCache{}
	root, _ := cache.Get("/", auth)
	_, err := root.GetChildren(auth)
	if err != nil {
		t.Fatal(err)
	}

	if _, exists := root.Children["Documents"]; !exists {
		t.Fatal("Could not find documents folder.")
	}
}

func TestSubdirChildrenUpdate(t *testing.T) {
	cache := ItemCache{}
	documents, err := cache.Get("/Documents", auth)
	if err != nil {
		t.Fatal(err)
	}
	children, err := documents.GetChildren(auth)
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
	cache := ItemCache{}
	item, _ := cache.Get("/Documents", auth)
	item2, _ := cache.Get("/Documents", auth)
	if item != item2 {
		t.Fatalf("Pointers to cached items do not match: %p != %p\n", item, item2)
	}
	if item == nil {
		t.Fatal("Item was nil!")
	}
}

func TestCacheWriteAppend(t *testing.T) {
	cache := ItemCache{}
	text := "test"

	item, err := cache.Get("/Documents/README.md", auth)
	if err != nil {
		t.Fatal("Failed to fetch item:", err)
	}
	err = item.FetchContent(auth)
	if err != nil {
		t.Fatal("Failed to fetch item content:", err)
	}

	startLen := *item.Size
	endLen := *item.Size + uint64(len(text))
	writeLen, status := item.Write([]byte(text), int64(startLen))
	if status != fuse.OK {
		t.Fatal("Error during write:", status)
	}
	if int(writeLen) != len(text) {
		t.Fatalf("Write length did not match expected value: %d != %d\n",
			writeLen, len(text))
	}
	if *item.Size != endLen {
		t.Fatalf("Size was not updated to proper length during write: %d != %d\n",
			item.Size, endLen)
	}

	readItem, err := cache.Get("/Documents/README.md", auth)
	if err != nil {
		t.Fatal("Failed to fetch item:", err)
	}

	if *readItem.Size != endLen {
		t.Fatalf("Size does not reflect updated file, "+
			"did the catch fetch an old copy of the item?: %d != %d\n",
			item.Size, endLen)
	}

	//TODO this test is just plain wrong and does not reflect how fuse does reads
	/*
		readResult := make([]byte, len(text))
		readItem.Read(readResult, int64(startLen))
		if string(readResult) != text {
			t.Fatalf("Unexpected read result \"%s\" != \"%s\"\n",
				string(readResult), text)
		}
	*/
}
