package main

import (
	"errors"
	"fmt"

	"restic"
	"restic/debug"
	"restic/filter"
)

type CmdRestore struct {
	Exclude []string `short:"e" long:"exclude" description:"Exclude a pattern (can be specified multiple times)"`
	Include []string `short:"i" long:"include" description:"Include a pattern, exclude everything else (can be specified multiple times)"`
	Target  string   `short:"t" long:"target"  description:"Directory to restore to"`

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
	return "snapshot-ID"
}

func (cmd CmdRestore) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("wrong number of arguments, Usage: %s", cmd.Usage())
	}

	if cmd.Target == "" {
		return errors.New("please specify a directory to restore to (--target)")
	}

	if len(cmd.Exclude) > 0 && len(cmd.Include) > 0 {
		return errors.New("exclude and include patterns are mutually exclusive")
	}

	snapshotIDString := args[0]

	debug.Log("restore", "restore %v to %v", snapshotIDString, cmd.Target)

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	if !cmd.global.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	id, err := restic.FindSnapshot(repo, snapshotIDString)
	if err != nil {
		cmd.global.Exitf(1, "invalid id %q: %v", snapshotIDString, err)
	}

	res, err := restic.NewRestorer(repo, id)
	if err != nil {
		cmd.global.Exitf(2, "creating restorer failed: %v\n", err)
	}

	res.Error = func(dir string, node *restic.Node, err error) error {
		cmd.global.Warnf("error for %s: %+v\n", dir, err)
		return nil
	}

	selectExcludeFilter := func(item string, dstpath string, node *restic.Node) bool {
		matched, err := filter.List(cmd.Exclude, item)
		if err != nil {
			cmd.global.Warnf("error for exclude pattern: %v", err)
		}

		return !matched
	}

	selectIncludeFilter := func(item string, dstpath string, node *restic.Node) bool {
		matched, err := filter.List(cmd.Include, item)
		if err != nil {
			cmd.global.Warnf("error for include pattern: %v", err)
		}

		return matched
	}

	if len(cmd.Exclude) > 0 {
		res.SelectFilter = selectExcludeFilter
	} else if len(cmd.Include) > 0 {
		res.SelectFilter = selectIncludeFilter
	}

	cmd.global.Verbosef("restoring %s to %s\n", res.Snapshot(), cmd.Target)

	err = res.RestoreTo(cmd.Target)
	if err != nil {
		return err
	}

	return nil
}
