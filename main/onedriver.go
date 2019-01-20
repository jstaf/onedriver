package main

import (
	"fmt"
	"log"

	"github.com/jstaf/onedriver/onedriver"
)

func main() {
	auth := onedriver.Authenticate()
	children, err := onedriver.GetChildren("/", auth)
	if err != nil {
		log.Fatal(err)
	}
	for _, item := range children {
		fmt.Println(item.Name)
	}

}
