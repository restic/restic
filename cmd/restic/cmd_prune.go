package main

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

const DefaultRepackThreshold int = 20

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
	RepackThreshold   int
}

var pruneOptions PruneOptions

func init() {
	cmdRoot.AddCommand(cmdPrune)

	f := cmdPrune.Flags()
	f.IntVarP(&pruneOptions.RepackThreshold, "repack-threshold", "", DefaultRepackThreshold, "only rebuild packs with at least `n`% unused space")

	f.SortFlags = false
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

	err := repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	var stats struct {
		totalFiles		int
		totalPacks		int
		totalBlobs		int
		totalBytes		uint64
		snapshots		int
		usedBlobs		int
		duplicateBlobs	int
		duplicateBytes	uint64
		remainingBytes	uint64
		removeBytes		uint64
	}

	Verbosef("counting files in repo\n")
	err = repo.List(ctx, restic.DataFile, func(restic.ID, int64) error {
		stats.totalFiles++
		return nil
	})
	if err != nil {
		return err
	}

	Verbosef("building new index for repo\n")
	bar := newProgressMax(!gopts.Quiet, uint64(stats.totalFiles), "packs")
	idx, invalidFiles, err := index.New(ctx, repo, restic.NewIDSet(), bar)
	if err != nil {
		return err
	}

	for _, id := range invalidFiles {
		Warnf("incomplete pack file (will be removed): %v\n", id)
	}

	blobCounts := make(map[restic.BlobHandle]int)
	for _, pack := range idx.Packs {
		stats.totalPacks += 1
		stats.totalBytes += uint64(pack.Size)
		for _, blob := range pack.Entries {
			stats.totalBlobs += 1
			h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
			blobCounts[h]++
			if blobCounts[h] > 1 {
				stats.duplicateBlobs++
				stats.duplicateBytes += uint64(blob.Length)
				stats.removeBytes += uint64(blob.Length)
			}
		}
	}
	Verbosef("repository contains %v packs (%v blobs) with %v\n",
		stats.totalPacks, stats.totalBlobs, formatBytes(stats.totalBytes))
	Verbosef("found %d duplicate blobs, %v duplicate\n",
		stats.duplicateBlobs, formatBytes(stats.duplicateBytes))

	Verbosef("load all snapshots\n")
	snapshots, err := restic.LoadAllSnapshots(ctx, repo)
	if err != nil {
		return err
	}
	stats.snapshots = len(snapshots)

	Verbosef("find data that is still in use for %d snapshots\n", stats.snapshots)

	usedBlobs := restic.NewBlobSet()
	seenBlobs := restic.NewBlobSet()
	bar = newProgressMax(!gopts.Quiet, uint64(stats.snapshots), "snapshots")
	bar.Start()
	for _, sn := range snapshots {
		debug.Log("process snapshot %v", sn.ID())

		err = restic.FindUsedBlobs(ctx, repo, *sn.Tree, usedBlobs, seenBlobs)
		if err != nil {
			if repo.Backend().IsNotExist(err) {
				return errors.Fatal("unable to load a tree from the repo: " + err.Error())
			}
			return err
		}

		debug.Log("processed snapshot %v", sn.ID())
		bar.Report(restic.Stat{Blobs: 1})
	}
	bar.Done()
	stats.usedBlobs = len(usedBlobs)

	Verbosef("found %d of %d data blobs still in use, %d blobs unused\n",
		stats.usedBlobs, stats.totalBlobs, stats.totalBlobs-stats.usedBlobs)

	if stats.usedBlobs > stats.totalBlobs {
		return errors.Fatalf("number of used blobs is larger than number of available blobs!\n" +
			"Please report this error (along with the output of the 'prune' run) at\n" +
			"https://github.com/restic/restic/issues/new")
	}

	// get packs to be removed
	removePacks := restic.NewIDSet()
	for _, id := range invalidFiles {
		removePacks.Insert(id)
	}

	// find packs that need a rewrite
	rewritePacks := restic.NewIDSet()
	for _, pack := range idx.Packs {
		packNeedsRewrite := false
		packNeedsRemoval := false

		if mixedBlobs(pack.Entries) {
			Verbosef("found deprecated mixed data/tree pack %v, marking for rewrite\n", pack.ID)
			packNeedsRewrite = true
		}

		packTotalBytes := uint64(0)
		packUnusedBytes := uint64(0)
		for _, blob := range pack.Entries {
			packTotalBytes += uint64(blob.Length)
			h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
			if !usedBlobs.Has(h) {
				packUnusedBytes += uint64(blob.Length)
			}

			// if pack has a duplicated blob, force rewrite
			if blobCounts[h] > 1 {
				packNeedsRewrite = true
			}
		}

		if packUnusedBytes >= packTotalBytes {
			packNeedsRemoval = true
		} else if packUnusedBytes > 0 {
			unusedPercent := int((packUnusedBytes * 100) / packTotalBytes)
			if unusedPercent >= opts.RepackThreshold {
				packNeedsRewrite = true
			}
		}

		if packNeedsRemoval {
			removePacks.Insert(pack.ID)
			stats.removeBytes += packTotalBytes
		} else if packNeedsRewrite {
			rewritePacks.Insert(pack.ID)
			stats.removeBytes += packUnusedBytes
		} else {
			stats.remainingBytes += packUnusedBytes
		}
	}

	Verbosef("will delete %d packs and rewrite %d packs\n",
		len(removePacks), len(rewritePacks))
	Verbosef("frees %s with %s unused remaining\n",
		formatBytes(stats.removeBytes), formatBytes(stats.remainingBytes))

	var obsoletePacks restic.IDSet
	if len(rewritePacks) != 0 {
		bar = newProgressMax(!gopts.Quiet, uint64(len(rewritePacks)), "packs rewritten")
		bar.Start()
		obsoletePacks, err = repository.Repack(ctx, repo, rewritePacks, usedBlobs, bar)
		if err != nil {
			return err
		}
		bar.Done()

		removePacks.Merge(obsoletePacks)
	}

	if len(rewritePacks) != 0 || len(removePacks) != 0 {
		if err = rebuildIndex(ctx, repo, removePacks); err != nil {
			return err
		}
	}

	if len(removePacks) != 0 {
		bar = newProgressMax(!gopts.Quiet, uint64(len(removePacks)), "packs deleted")
		bar.Start()
		for packID := range removePacks {
			h := restic.Handle{Type: restic.DataFile, Name: packID.String()}
			err = repo.Backend().Remove(ctx, h)
			if err != nil {
				Warnf("unable to remove file %v from the repository\n", packID.Str())
			}
			bar.Report(restic.Stat{Blobs: 1})
		}
		bar.Done()
	}

	Verbosef("done\n")
	return nil
}
