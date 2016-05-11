package main

import "restic/repository"

type CmdRebuildIndex struct {
	global *GlobalOptions

	repo *repository.Repository
}

func init() {
	_, err := parser.AddCommand("rebuild-index",
		"rebuild the index",
		"The rebuild-index command builds a new index",
		&CmdRebuildIndex{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdRebuildIndex) Execute(args []string) error {
	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}
	cmd.repo = repo

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return repository.RebuildIndex(repo)
}
