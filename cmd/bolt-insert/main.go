package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	bolt "github.com/etcd-io/bbolt"
	flag "github.com/spf13/pflag"
)

func main() {
	flag.Parse()
	if len(flag.Args()) < 3 {
		fmt.Println("Usage: bolt-insert file bucket key < data")
		os.Exit(1)
	}

	bucket := flag.Arg(1)
	key := flag.Arg(2)
	fmt.Printf("Writing to %s/%s...\n", bucket, key)

	db, err := bolt.Open(flag.Arg(0), 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	contents, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucketIfNotExists([]byte(bucket))
		return b.Put([]byte(key), contents)
	})
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("success!")
}
