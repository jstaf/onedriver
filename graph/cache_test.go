// these tests are independent of the mounted fs
package graph

import (
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
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
	if documents.Name != "Documents" {
		t.Fatalf("Failed to fetch \"/Documents\". Got \"%s\" instead!\n", documents.Name)
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

func TestCacheWriteAppend(t *testing.T) {
	// skip for now
	t.SkipNow()

	cache := NewCache(auth)
	text := "test"

	// copy our README.md into the cache
	documents, _ := cache.Get("/Documents", auth)
	newItem := NewDriveItem("README.md", 0644, documents)
	content, _ := ioutil.ReadFile("README.md")
	newItem.data = &content
	cache.Insert("/Documents/README.md", auth, newItem)

	item, err := cache.Get("/Documents/README.md", auth)
	if err != nil {
		t.Fatal("Failed to fetch item:", err)
	}
	err = item.FetchContent(auth)
	if err != nil {
		t.Fatal("Failed to fetch item content:", err)
	}

	startLen := item.Size
	endLen := item.Size + uint64(len(text))
	writeLen, status := item.Write([]byte(text), int64(startLen))
	if status != fuse.OK {
		t.Fatal("Error during write:", status)
	}
	if int(writeLen) != len(text) {
		t.Fatalf("Write length did not match expected value: %d != %d\n",
			writeLen, len(text))
	}
	if item.Size != endLen {
		t.Fatalf("Size was not updated to proper length during write: %d != %d\n",
			item.Size, endLen)
	}

	readItem, err := cache.Get("/Documents/README.md", auth)
	if err != nil {
		t.Fatal("Failed to fetch item:", err)
	}

	if readItem.Size != endLen {
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
