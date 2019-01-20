package main

import (
	"fmt"

	"github.com/jstaf/onedriver/onedriver"
)

func main() {
	auth := onedriver.Authenticate()
	resp, err := onedriver.Request("/me/drive/root", auth, "GET", nil)
	fmt.Printf("%s\n%v\n", resp, err)
}
