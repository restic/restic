package main

import (
	"fmt"
	"path/filepath"

	"github.com/restic/restic"
	"github.com/restic/restic/debug"
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

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	id, err := restic.FindSnapshot(repo, args[0])
	if err != nil {
		cmd.global.Exitf(1, "invalid id %q: %v", args[0], err)
	}

	target := args[1]

	// create restorer
	res, err := restic.NewRestorer(repo, id)
	if err != nil {
		cmd.global.Exitf(2, "creating restorer failed: %v\n", err)
	}

	res.Error = func(dir string, node *restic.Node, err error) error {
		cmd.global.Warnf("error for %s: %+v\n", dir, err)

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
		pattern := args[2]
		cmd.global.Verbosef("filter pattern %q\n", pattern)

		res.SelectForRestore = func(item string, dstpath string, node *restic.Node) bool {
			matched, err := filepath.Match(pattern, node.Name)
			if err != nil {
				panic(err)
			}
			if !matched {
				debug.Log("restic.restore", "item %v doesn't match pattern %q", item, pattern)
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
