package main

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/walker"
)

// findBenchPrinter is a no-op statefulOutput printer. The benchmark pattern
// never matches, so output is irrelevant; a no-op sink keeps printer overhead
// out of the timings and allocation counts.
type findBenchPrinter struct{}

func (findBenchPrinter) S(string, ...interface{}) {}
func (findBenchPrinter) P(string, ...interface{}) {}
func (findBenchPrinter) E(string, ...interface{}) {}

// newFindBenchFinder builds a Finder that discards all output.
func newFindBenchFinder(repo restic.Repository, patterns ...string) *Finder {
	printer := findBenchPrinter{}
	return &Finder{
		repo: repo,
		pat: findPattern{
			pattern: patterns,
		},
		out: statefulOutput{
			printer: printer,
			stdout:  io.Discard,
		},
		printer: printer,
	}
}

// benchmarkFindPatternInverted times the live inverted traversal over snapshots.
func benchmarkFindPatternInverted(b *testing.B, repo restic.Repository, snapshots []*data.Snapshot, pattern string) {
	b.Helper()
	b.ReportAllocs()

	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		finder := newFindBenchFinder(repo, pattern)
		rtest.OK(b, finder.findPatternInverted(ctx, snapshots))
	}
}

// findPatternPerSnapshot is a benchmark-only per-snapshot baseline. The
// production per-snapshot path was removed; this walks each snapshot's tree
// independently via walker.Walk, applying the same match/prune logic as the
// inverted path so the two are comparable. It lives only in this test file.
func findPatternPerSnapshot(ctx context.Context, f *Finder, snapshots []*data.Snapshot) error {
	for _, sn := range snapshots {
		if sn.Tree == nil {
			return fmt.Errorf("snapshot %v has no tree", sn.ID().Str())
		}
		f.out.newsn = sn
		err := walker.Walk(ctx, f.repo, *sn.Tree, walker.WalkVisitor{
			ProcessNode: func(_ restic.ID, nodePath string, node *data.Node, err error) error {
				if err != nil {
					f.printer.S("Unable to load tree for %s", nodePath)
					return walker.ErrSkipNode
				}
				if node == nil {
					return nil
				}
				if node.Type == data.NodeTypeInvalid {
					return fmt.Errorf("node type is empty for node %q", node.Name)
				}
				foundMatch, childMayMatch, err := f.matchFindPattern(nodePath, node)
				if err != nil {
					return err
				}
				if foundMatch && f.matchFindNodeTimeRange(node) {
					f.out.PrintPattern(nodePath, node)
				}
				if node.Type == data.NodeTypeDir && !childMayMatch {
					return walker.ErrSkipNode
				}
				return nil
			},
		})
		if err != nil {
			return err
		}
	}
	f.out.Finish()
	return nil
}

// benchmarkFindPatternPerSnapshot times the benchmark-only per-snapshot walk.
func benchmarkFindPatternPerSnapshot(b *testing.B, repo restic.Repository, snapshots []*data.Snapshot, pattern string) {
	b.Helper()
	b.ReportAllocs()

	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		finder := newFindBenchFinder(repo, pattern)
		rtest.OK(b, findPatternPerSnapshot(ctx, finder, snapshots))
	}
}

// buildHighReuseSnapshots builds count snapshots sharing one base tree, so the
// inverted walk loads each subtree once for the whole run.
func buildHighReuseSnapshots(b testing.TB, repo restic.Repository, count int) []*data.Snapshot {
	base := data.TestCreateSnapshot(b, repo, time.Unix(1710000000, 0), 3)

	snapshots := make([]*data.Snapshot, 0, count)
	snapshots = append(snapshots, base)
	for i := 1; i < count; i++ {
		snapshots = append(snapshots, saveSnapshotWithTree(b, repo, *base.Tree, base.Time.Add(time.Duration(i)*time.Second)))
	}

	return snapshots
}

// buildLowReuseSnapshots builds count snapshots with independent trees, so no
// cross-snapshot dedup occurs.
func buildLowReuseSnapshots(b testing.TB, repo restic.Repository, count int) []*data.Snapshot {
	snapshots := make([]*data.Snapshot, 0, count)
	for i := 0; i < count; i++ {
		snapshots = append(snapshots, data.TestCreateSnapshot(b, repo, time.Unix(1711000000+int64(i), 0), 3))
	}

	return snapshots
}

// buildMovedPathSnapshots builds count snapshots that each mount the same shared
// leaf subtree at a different path, exercising path-keyed bucketing.
func buildMovedPathSnapshots(b testing.TB, repo restic.Repository, count int) []*data.Snapshot {
	rootIDs := make([]restic.ID, 0, count)

	err := repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		sharedTreeID := data.TestSaveNodes(b, ctx, uploader, []*data.Node{
			{
				Name: "needle.txt",
				Type: data.NodeTypeFile,
				Mode: 0644,
				Size: 1,
			},
		})

		for i := 0; i < count; i++ {
			dirName := fmt.Sprintf("dir-%03d", i)
			rootID := data.TestSaveNodes(b, ctx, uploader, []*data.Node{
				{
					Name:    dirName,
					Type:    data.NodeTypeDir,
					Mode:    0755,
					Subtree: &sharedTreeID,
				},
			})
			rootIDs = append(rootIDs, rootID)
		}
		return nil
	})
	rtest.OK(b, err)

	snapshots := make([]*data.Snapshot, 0, len(rootIDs))
	for i, rootID := range rootIDs {
		snapshotTime := time.Unix(1712000000+int64(i), 0)
		snapshots = append(snapshots, saveSnapshotWithTree(b, repo, rootID, snapshotTime))
	}

	return snapshots
}

// buildScaleSnapshots builds a high-sharing scale fixture: snapCount snapshots
// all referencing one shared root tree holding dirCount directories, each with a
// leaf file. snapCount*dirCount attributions exceed the old 200k cache cap.
func buildScaleSnapshots(b testing.TB, repo restic.Repository, snapCount, dirCount int) []*data.Snapshot {
	var root restic.ID
	err := repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		dirs := make([]*data.Node, 0, dirCount)
		for i := 0; i < dirCount; i++ {
			leafID := data.TestSaveNodes(b, ctx, uploader, []*data.Node{
				{Name: "needle.txt", Type: data.NodeTypeFile, Mode: 0644, Size: 1},
			})
			dirName := fmt.Sprintf("dir-%05d", i)
			dirs = append(dirs, &data.Node{
				Name:    dirName,
				Type:    data.NodeTypeDir,
				Mode:    0755,
				Subtree: &leafID,
			})
		}
		root = data.TestSaveNodes(b, ctx, uploader, dirs)
		return nil
	})
	rtest.OK(b, err)

	base := time.Unix(1713000000, 0)
	snapshots := make([]*data.Snapshot, snapCount)
	for i := range snapshots {
		snapshots[i] = saveSnapshotWithTree(b, repo, root, base.Add(time.Duration(i)*time.Second))
	}
	return snapshots
}

// BenchmarkFindPattern benchmarks the inverted pattern traversal against a
// benchmark-only per-snapshot baseline across reuse shapes plus a scale case.
func BenchmarkFindPattern(b *testing.B) {
	const snapshotCount = 40

	scenarios := []struct {
		name    string
		builder func(testing.TB, restic.Repository, int) []*data.Snapshot
	}{
		{name: "HighReuse", builder: buildHighReuseSnapshots},
		{name: "LowReuse", builder: buildLowReuseSnapshots},
		{name: "MovedPaths", builder: buildMovedPathSnapshots},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			repo := repository.TestRepository(b)
			snapshots := scenario.builder(b, repo, snapshotCount)

			b.Run("Inverted", func(b *testing.B) {
				benchmarkFindPatternInverted(b, repo, snapshots, "definitely-not-here")
			})
			b.Run("PerSnapshot", func(b *testing.B) {
				benchmarkFindPatternPerSnapshot(b, repo, snapshots, "definitely-not-here")
			})
		})
	}

	// Scale: many snapshots x directories beyond the old 200k cap, high sharing.
	// The Matching variants use the pattern every leaf file matches, exercising
	// the per-snapshot match buffering (snapCount*dirCount attributions) that
	// the non-matching pattern never reaches.
	b.Run("Scale", func(b *testing.B) {
		const (
			snapCount = 200
			dirCount  = 1200 // 200 x 1200 = 240k attributions
		)
		repo := repository.TestRepository(b)
		snapshots := buildScaleSnapshots(b, repo, snapCount, dirCount)

		b.Run("Inverted", func(b *testing.B) {
			benchmarkFindPatternInverted(b, repo, snapshots, "definitely-not-here")
		})
		b.Run("PerSnapshot", func(b *testing.B) {
			benchmarkFindPatternPerSnapshot(b, repo, snapshots, "definitely-not-here")
		})
		b.Run("InvertedMatching", func(b *testing.B) {
			benchmarkFindPatternInverted(b, repo, snapshots, "needle.txt")
		})
		b.Run("PerSnapshotMatching", func(b *testing.B) {
			benchmarkFindPatternPerSnapshot(b, repo, snapshots, "needle.txt")
		})
	})
}
