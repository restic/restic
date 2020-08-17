package main

import (
	"io"

	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/restic"
)

const numWarmUpWorkers = 8

// WarmUpFiles opens all files the given fileList in parallel in order to "warm" them up
// This can be useful when accessing cold storage
func WarmUpFiles(gopts GlobalOptions, fileList restic.IDSet, fileType restic.FileType) error {
	totalCount := len(fileList)
	fileChan := make(chan restic.ID)
	go func() {
		for id := range fileList {
			fileChan <- id
		}
		close(fileChan)
	}()

	Verbosef("reopen repository\n")
	// reopen the backend to get one without retries
	be, err := open(gopts.Repo, gopts, gopts.extended)
	if err != nil {
		return err
	}

	bar := newProgressMax(!gopts.JSON && !gopts.Quiet, uint64(totalCount), "files warmed-up")
	wg, ctx := errgroup.WithContext(gopts.ctx)
	defer bar.Done()
	for i := 0; i < numWarmUpWorkers; i++ {
		wg.Go(func() error {
			for id := range fileChan {
				h := restic.Handle{Type: fileType, Name: id.String()}
				// ignore errors, as we expect them for cold data loads
				_ = be.Load(ctx, h, 1, 0, func(_ io.Reader) error { return nil })
				if !gopts.JSON {
					Verboseff("warmed up %v\n", h)
				}
				bar.Add(1)
			}
			return nil
		})
	}

	return wg.Wait()
}
