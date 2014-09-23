package main

import (
	"fmt"
	"os"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

func commandInit(path string) error {
	pw := read_password("enter password for new backend: ")
	pw2 := read_password("enter password again: ")

	if pw != pw2 {
		errx(1, "passwords do not match")
	}

	be, err := backend.CreateLocal(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating local backend at %s failed: %v\n", path, err)
		os.Exit(1)
	}

	_, err = khepri.CreateKey(be, pw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating key in local backend at %s failed: %v\n", path, err)
		os.Exit(1)
	}

	fmt.Printf("created khepri backend at %s\n", be.Location())

	return nil
}
