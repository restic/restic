package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

func commandBackup(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: backup [dir|file]")
	}

	target := args[0]

	arch, err := khepri.NewArchiver(be, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %v\n", err)
	}
	arch.Error = func(dir string, fi os.FileInfo, err error) error {
		fmt.Fprintf(os.Stderr, "error for %s: %v\n%s\n", dir, err, fi)
		return err
	}

	_, blob, err := arch.Import(target)
	if err != nil {
		return err
	}

	fmt.Printf("snapshot %s saved\n", blob.Storage)

	return nil
}
