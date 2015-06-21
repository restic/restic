package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/restic/restic"
)

type CmdRestore struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("restore",
		"restore a snapshot",
		"The restore command restores a snapshot to a directory",
		&CmdRestore{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdRestore) Usage() string {
	return "snapshot-ID TARGETDIR [PATTERN]"
}

func (cmd CmdRestore) Execute(args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("wrong number of arguments, Usage: %s", cmd.Usage())
	}

	s, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	err = s.LoadIndex()
	if err != nil {
		return err
	}

	id, err := restic.FindSnapshot(s, args[0])
	if err != nil {
		cmd.global.Exitf(1, "invalid id %q: %v", args[0], err)
	}

	target := args[1]

	// create restorer
	res, err := restic.NewRestorer(s, id)
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

	// TODO: a filter against the full path sucks as filepath.Match doesn't match
	// directory separators on '*'. still, it's better than nothing.
	if len(args) > 2 {
		res.Filter = func(item string, dstpath string, node *restic.Node) bool {
			matched, err := filepath.Match(item, args[2])
			if err != nil {
				panic(err)
			}
			return matched
		}
	}

	cmd.global.Verbosef("restoring %s to %s\n", res.Snapshot(), target)

	err = res.RestoreTo(target)
	if err != nil {
		return err
	}

	return nil
}
