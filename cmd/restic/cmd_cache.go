package main

import (
	"fmt"

	"github.com/restic/restic"
)

type CmdCache struct{}

func init() {
	_, err := parser.AddCommand("cache",
		"manage cache",
		"The cache command creates and manages the local cache",
		&CmdCache{})
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

	s, err := OpenRepo()
	if err != nil {
		return err
	}

	cache, err := restic.NewCache(s, opts.CacheDir)
	if err != nil {
		return err
	}

	fmt.Printf("clear cache for old snapshots\n")
	err = cache.Clear(s)
	if err != nil {
		return err
	}
	fmt.Printf("done\n")

	return nil
}
