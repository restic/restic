package main

import (
	"fmt"

	"restic"
)

type CmdCache struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("cache",
		"manage cache",
		"The cache command creates and manages the local cache",
		&CmdCache{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdCache) Usage() string {
	return "[update|clear]"
}

func (cmd CmdCache) Execute(args []string) error {
	// if len(args) == 0 || len(args) > 2 {
	// 	return fmt.Errorf("wrong number of parameters, Usage: %s", cmd.Usage())
	// }

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	cache, err := restic.NewCache(repo, cmd.global.CacheDir)
	if err != nil {
		return err
	}

	fmt.Printf("clear cache for old snapshots\n")
	err = cache.Clear(repo)
	if err != nil {
		return err
	}
	fmt.Printf("done\n")

	return nil
}
