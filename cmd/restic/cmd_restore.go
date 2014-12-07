package main

import (
	"fmt"
	"os"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
)

type CmdRestore struct{}

func init() {
	_, err := parser.AddCommand("restore",
		"restore a snapshot",
		"The restore command restores a snapshot to a directory",
		&CmdRestore{})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdRestore) Usage() string {
	return "snapshot-ID TARGETDIR"
}

func (cmd CmdRestore) Execute(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("wrong number of arguments, Usage: %s", cmd.Usage())
	}

	be, key, err := OpenRepo()
	if err != nil {
		return err
	}

	id, err := backend.FindSnapshot(be, args[0])
	if err != nil {
		errx(1, "invalid id %q: %v", args[0], err)
	}

	target := args[1]

	// create restorer
	res, err := restic.NewRestorer(be, key, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating restorer failed: %v\n", err)
		os.Exit(2)
	}

	res.Error = func(dir string, node *restic.Node, err error) error {
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
