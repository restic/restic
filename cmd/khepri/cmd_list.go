package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/fd0/khepri"
)

func commandList(repo *khepri.Repository, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: list [blob|ref]")
	}

	tpe := khepri.NewTypeFromString(args[0])

	ids, err := repo.List(tpe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return nil
	}

	for _, id := range ids {
		fmt.Printf("%v\n", id)
	}

	return nil
}
