package main

import "restic"

type CmdUnlock struct {
	RemoveAll bool `long:"remove-all" description:"Remove all locks, even stale ones"`

	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("unlock",
		"remove locks",
		"The unlock command checks for stale locks and removes them",
		&CmdUnlock{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdUnlock) Usage() string {
	return "[unlock-options]"
}

func (cmd CmdUnlock) Execute(args []string) error {
	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	fn := restic.RemoveStaleLocks
	if cmd.RemoveAll {
		fn = restic.RemoveAllLocks
	}

	err = fn(repo)
	if err != nil {
		return err
	}

	cmd.global.Verbosef("successfully removed locks\n")
	return nil
}
