package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
	tomb "gopkg.in/tomb.v2"
)

var cmdPrune = &cobra.Command{
	Use:   "prune [flags]",
	Short: "Remove unneeded data from the repository",
	Long: `
The "prune" command checks the repository and removes data that is not
referenced and therefore not needed any more.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPrune(pruneOptions, globalOptions)
	},
}

// PruneOptions collects all options for the prune command.
type PruneOptions struct {
	IgnoreIndex bool
}

var pruneOptions PruneOptions

func init() {
	cmdRoot.AddCommand(cmdPrune)

	f := cmdPrune.Flags()
	f.BoolVar(&pruneOptions.IgnoreIndex, "ignore-index", false, "rebuild index before pruning")
}

func shortenStatus(maxLength int, s string) string {
	if len(s) <= maxLength {
		return s
	}

	if maxLength < 3 {
		return s[:maxLength]
	}

	return s[:maxLength-3] + "..."
}

// newProgressMax returns a progress that counts blobs.
func newProgressMax(show bool, max uint64, description string) *restic.Progress {
	if !show {
		return nil
	}

	if max == 0 {
		return nil
	}

	p := restic.NewProgress()

	p.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		status := fmt.Sprintf("[%s] %s  %d / %d %s",
			formatDuration(d),
			formatPercent(s.Blobs, max),
			s.Blobs, max, description)

		if w := stdoutTerminalWidth(); w > 0 {
			status = shortenStatus(w, status)
		}

		PrintProgress("%s", status)
	}

	p.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\n")
	}

	return p
}

func newProgress(show bool, description string) *restic.Progress {
	if !show {
		return nil
	}

	p := restic.NewProgress()

	p.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		status := fmt.Sprintf("[%s] %d %s",
			formatDuration(d),
			s.Blobs, description)

		if w := stdoutTerminalWidth(); w > 0 {
			status = shortenStatus(w, status)
		}

		PrintProgress("%s", status)
	}

	p.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\n")
	}

	return p
}

func runPrune(opts PruneOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return pruneRepository(opts, gopts, repo)
}

func mixedBlobs(list []restic.Blob) bool {
	var tree, data bool

	for _, pb := range list {
		switch pb.Type {
		case restic.TreeBlob:
			tree = true
		case restic.DataBlob:
			data = true
		}

		if tree && data {
			return true
		}
	}

	return false
}

func pruneRepository(opts PruneOptions, gopts GlobalOptions, repo restic.Repository) error {
	ctx := gopts.ctx

	var stats struct {
		indexes   int
		blobs     int
		packs     int
		snapshots int
		bytes     int64
	}
	type Pack struct {
		ID   restic.ID
		Size int64
	}
	indexFiles := restic.NewIDSet()
	packFiles := make(map[restic.ID]Pack)

	Verbosef("listing files in repo\n")
	err := repo.List(ctx, restic.IndexFile, func(id restic.ID, size int64) error {
		indexFiles.Insert(id)
		stats.indexes++
		return nil
	})
	err = repo.List(ctx, restic.DataFile, func(id restic.ID, size int64) error {
		stats.packs++
		stats.bytes += size
		packFiles[id] = Pack{ID: id, Size: size}
		return nil
	})
	if err != nil {
		return err
	}

	var idx *index.Index
	var invalidFiles restic.IDs
	if opts.IgnoreIndex {
		Verbosef("building index for repo\n")
		bar := newProgressMax(!gopts.Quiet, uint64(stats.packs), "packs")

		idx, invalidFiles, err = index.New(ctx, repo, nil, bar)
		if err != nil {
			return err
		}
	} else {
		Verbosef("loading index for repo\n")
		bar := newProgressMax(!gopts.Quiet, uint64(stats.indexes), "index files")

		idx, err = index.Load(ctx, repo, bar)
		if err != nil {
			return err
		}
	}

	for _, pack := range idx.Packs {
		if _, ok := packFiles[pack.ID]; ok {
			// We're going to be using packFiles later to examine
			// packs that aren't in the index, so remove this pack
			// file from packFiles so we know it's already handled.
			delete(packFiles, pack.ID)
		} else {
			// This pack is in the index, but isn't actually in the
			// repository. Remove it from the index so everything
			// knows it doesn't exist any more.
			delete(idx.Packs, pack.ID)
		}
	}

	Verbosef("checking for packs not in index\n")
	bar := newProgressMax(!gopts.Quiet, uint64(len(packFiles)), "packs")
	bar.Start()

	t, wctx := tomb.WithContext(ctx)

	inputCh := make(chan Pack)
	invalidPacksCh := make(chan restic.ID)

	inputWorker := func() error {
		for _, packFile := range packFiles {
			select {
			case inputCh <- packFile:
			case <-t.Dying():
				close(inputCh)
				return tomb.ErrDying
			}
		}
		close(inputCh)
		return nil
	}

	var wg sync.WaitGroup
	scanUnknownPacksWorker := func() error {
		defer wg.Done()
		for packFile := range inputCh {
			entries, size, err := repo.ListPack(wctx, packFile.ID, packFile.Size)
			if err != nil {
				cause := errors.Cause(err)
				if _, ok := cause.(pack.InvalidFileError); ok {
					select {
					case invalidPacksCh <- packFile.ID:
					case <-t.Dying():
						return tomb.ErrDying
					}
					bar.Report(restic.Stat{Blobs: 1})
				}
				Printf("pack file cannot be listed: %v: %v\n", packFile.ID, err)
				bar.Report(restic.Stat{Blobs: 1})
				continue
			}

			err = idx.AddPack(packFile.ID, size, entries)
			if err != nil {
				Printf("couldn't add pack %v to index: %v\n", packFile.ID, err)
			}
			bar.Report(restic.Stat{Blobs: 1})
		}
		return nil
	}

	collectInvalidPacksWorker := func() error {
		for {
			select {
			case file, ok := <-invalidPacksCh:
				if !ok {
					return nil
				}
				invalidFiles = append(invalidFiles, file)
			case <-t.Dying():
				return tomb.ErrDying
			}
		}
	}

	t.Go(func() error {
		t.Go(inputWorker)
		count := runtime.NumCPU()
		wg.Add(count)
		for i := 0; i < count; i++ {
			t.Go(scanUnknownPacksWorker)
		}

		t.Go(func() error {
			wg.Wait()
			close(invalidPacksCh)
			return nil
		})

		t.Go(collectInvalidPacksWorker)
		return nil
	})
	err = t.Wait()
	bar.Done()

	for _, pack := range idx.Packs {
		stats.blobs += len(pack.Entries)
		for _, blob := range pack.Entries {
			repo.Index().Store(restic.PackedBlob{Blob: blob, PackID: pack.ID})
		}
	}

	for _, id := range invalidFiles {
		Warnf("incomplete pack file (will be removed): %v\n", id)
	}

	Verbosef("repository contains %v packs (%v blobs) with %v\n",
		stats.packs, stats.blobs, formatBytes(uint64(stats.bytes)))

	blobCount := make(map[restic.BlobHandle]int)
	var duplicateBlobs uint64
	var duplicateBytes uint64

	// find duplicate blobs
	for _, p := range idx.Packs {
		for _, entry := range p.Entries {
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			blobCount[h]++

			if blobCount[h] > 1 {
				duplicateBlobs++
				duplicateBytes += uint64(entry.Length)
			}
		}
	}

	Verbosef("processed %d blobs: %d duplicate blobs, %v duplicate\n",
		stats.blobs, duplicateBlobs, formatBytes(duplicateBytes))
	Verbosef("load all snapshots\n")

	// find referenced blobs
	snapshots, err := restic.LoadAllSnapshots(ctx, repo)
	if err != nil {
		return err
	}

	stats.snapshots = len(snapshots)

	Verbosef("find data that is still in use for %d snapshots\n", stats.snapshots)

	bar = newProgressMax(!gopts.Quiet, uint64(stats.snapshots), "snapshots")
	usedBlobs, err := restic.FindUsedBlobs(ctx, repo, snapshots, bar)
	if err != nil {
		return err
	}

	if len(usedBlobs) > stats.blobs {
		return errors.Fatalf("number of used blobs is larger than number of available blobs!\n" +
			"Please report this error (along with the output of the 'prune' run) at\n" +
			"https://github.com/restic/restic/issues/new")
	}

	Verbosef("found %d of %d data blobs still in use, removing %d blobs\n",
		len(usedBlobs), stats.blobs, stats.blobs-len(usedBlobs))

	// find packs that need a rewrite
	rewritePacks := restic.NewIDSet()
	for _, pack := range idx.Packs {
		if mixedBlobs(pack.Entries) {
			rewritePacks.Insert(pack.ID)
			continue
		}

		for _, blob := range pack.Entries {
			h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
			if !usedBlobs.Has(h) {
				rewritePacks.Insert(pack.ID)
				continue
			}

			if blobCount[h] > 1 {
				rewritePacks.Insert(pack.ID)
			}
		}
	}

	removeBytes := duplicateBytes

	// find packs that are unneeded
	removePacks := restic.NewIDSet()

	Verbosef("will remove %d invalid files\n", len(invalidFiles))
	for _, id := range invalidFiles {
		removePacks.Insert(id)
	}

	for packID, p := range idx.Packs {
		hasActiveBlob := false
		for _, blob := range p.Entries {
			h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
			if usedBlobs.Has(h) {
				hasActiveBlob = true
				continue
			}

			removeBytes += uint64(blob.Length)
		}

		if hasActiveBlob {
			continue
		}

		removePacks.Insert(packID)

		if !rewritePacks.Has(packID) {
			return errors.Fatalf("pack %v is unneeded, but not contained in rewritePacks", packID.Str())
		}

		rewritePacks.Delete(packID)
	}

	Verbosef("will delete %d packs and rewrite %d packs, this frees %s\n",
		len(removePacks), len(rewritePacks), formatBytes(removeBytes))

	var obsoletePacks restic.IDSet
	if len(rewritePacks) != 0 {
		bar = newProgressMax(!gopts.Quiet, uint64(len(rewritePacks)), "packs rewritten")
		obsoletePacks, err = repository.Repack(ctx, repo, rewritePacks, usedBlobs, bar)
		if err != nil {
			return err
		}

		knownPacks := restic.NewIDSet()
		for packID := range idx.Packs {
			knownPacks.Insert(packID)
		}
		for blob := range repo.Index().Each(ctx) {
			if _, ok := knownPacks[blob.PackID]; ok {
				continue
			}
			pack, ok := idx.Packs[blob.PackID]
			if !ok {
				pack.ID = blob.PackID
			}
			pack.Entries = append(pack.Entries, blob.Blob)
			idx.Packs[pack.ID] = pack
		}
	}

	removePacks.Merge(obsoletePacks)

	for packID := range removePacks {
		idx.RemovePack(packID)
	}
	var supersedes restic.IDs
	for id := range indexFiles {
		supersedes = append(supersedes, id)
	}

	Verbosef("saving new index\n")
	bar = newProgress(!gopts.Quiet, "index files")
	_, err = idx.Save(ctx, repo, supersedes, bar)
	if err != nil {
		return err
	}

	Verbosef("remove %d old index files\n", len(indexFiles))

	for id := range indexFiles {
		if err := repo.Backend().Remove(ctx, restic.Handle{
			Type: restic.IndexFile,
			Name: id.String(),
		}); err != nil {
			Warnf("error removing old index %v: %v\n", id.Str(), err)
		}
	}

	t, wctx = tomb.WithContext(ctx)
	removePacksCh := make(chan restic.ID)
	bar = newProgressMax(!gopts.Quiet, uint64(len(removePacks)), "packs deleted")
	deleteWorker := func() error {
		for packID := range removePacksCh {
			select {
			case <-t.Dying():
				break
			default:
			}

			h := restic.Handle{Type: restic.DataFile, Name: packID.String()}
			err = repo.Backend().Remove(wctx, h)
			if err != nil {
				Warnf("unable to remove file %v from the repository\n", packID.Str())
			}
			bar.Report(restic.Stat{Blobs: 1})
		}
		return nil
	}

	bar.Start()
	t.Go(func() error {
		t.Go(func() error {
			for packID := range removePacks {
				select {
				case removePacksCh <- packID:
				case <-t.Dying():
					break
				}
			}
			close(removePacksCh)
			return nil
		})

		deleteWorkers := repo.Backend().Connections()
		for i := uint(0); i < deleteWorkers; i++ {
			t.Go(deleteWorker)
		}

		return nil
	})
	err = t.Wait()
	bar.Done()
	if err != nil {
		return err
	}

	Verbosef("done\n")
	return nil
}
