package main

import (
	"fmt"
	"os"

	"github.com/fd0/khepri"
)

func commandInit(path string) error {
	repo, err := khepri.CreateRepository(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating repository at %s failed: %v\n", path, err)
		os.Exit(1)
	}

	fmt.Printf("created khepri repository at %s\n", repo.Path())

	return nil
}
