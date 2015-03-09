package main

import (
	"fmt"
	"io"
	"sync"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
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

	fmt.Printf("update cache, load trees\n")

	list, err := s.List(backend.Tree)
	if err != nil {
		return err
	}

	cache, err := restic.NewCache()
	if err != nil {
		return err
	}

	treeCh := make(chan backend.ID)
	worker := func(wg *sync.WaitGroup, ch chan backend.ID) {
		for treeID := range ch {
			cached, err := cache.Has(backend.Tree, "", treeID)
			if err != nil {
				fmt.Printf("tree %v cache error: %v\n", treeID.Str(), err)
				continue
			}

			if cached {
				fmt.Printf("tree %v already cached\n", treeID.Str())
				continue
			}

			rd, err := s.GetReader(backend.Tree, treeID)
			if err != nil {
				fmt.Printf("  load error: %v\n", err)
				continue
			}

			decRd, err := s.Key().DecryptFrom(rd)
			if err != nil {
				fmt.Printf("  store error: %v\n", err)
				continue
			}

			wr, err := cache.Store(backend.Tree, "", treeID)
			if err != nil {
				fmt.Printf("  store error: %v\n", err)
				continue
			}

			_, err = io.Copy(wr, decRd)
			if err != nil {
				fmt.Printf("  Copy error: %v\n", err)
				continue
			}

			err = decRd.Close()
			if err != nil {
				fmt.Printf("  close error: %v\n", err)
				continue
			}

			err = rd.Close()
			if err != nil {
				fmt.Printf("  close error: %v\n", err)
				continue
			}

			fmt.Printf("tree %v stored\n", treeID.Str())
		}
		wg.Done()
	}

	var wg sync.WaitGroup
	// start workers
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go worker(&wg, treeCh)
	}

	for _, treeID := range list {
		treeCh <- treeID
	}

	close(treeCh)

	wg.Wait()

	return nil
}
