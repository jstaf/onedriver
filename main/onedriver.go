package main

import (
	"fmt"

	"github.com/jstaf/onedriver/onedriver"
)

func main() {
	auth := onedriver.Authenticate()
	item, err := onedriver.GetItem("/kdfslkdsjlf", auth)
	fmt.Printf("%+v\n", item)
	fmt.Printf("%s\n", err)
	fmt.Println(err == nil)
}
