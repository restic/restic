package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

func commandRestore(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: restore ID dir")
	}

	id, err := backend.FindSnapshot(be, args[0])
	if err != nil {
		errx(1, "invalid id %q: %v", args[0], err)
	}

	target := args[1]

	// create restorer
	res, err := khepri.NewRestorer(be, key, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating restorer failed: %v\n", err)
		os.Exit(2)
	}

	res.Error = func(dir string, node *khepri.Node, err error) error {
		fmt.Fprintf(os.Stderr, "error for %s: %+v\n", dir, err)

		// if node.Type == "dir" {
		// 	if e, ok := err.(*os.PathError); ok {
		// 		if errn, ok := e.Err.(syscall.Errno); ok {
		// 			if errn == syscall.EEXIST {
		// 				fmt.Printf("ignoring already existing directory %s\n", dir)
		// 				return nil
		// 			}
		// 		}
		// 	}
		// }
		return err
	}

	fmt.Printf("restoring %s to %s\n", res.Snapshot(), target)

	err = res.RestoreTo(target)
	if err != nil {
		return err
	}

	return nil
}
