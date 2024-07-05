package archiver

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
)

// Scanner  traverses the targets and calls the function Result with cumulated
// stats concerning the files and folders found. Select is used to decide which
// items should be included. Error is called when an error occurs.
type Scanner struct {
	FS           fs.FS
	SelectByName SelectByNameFunc
	Select       SelectFunc
	Error        ErrorFunc
	Result       func(item string, s ScanStats)
}

// NewScanner initializes a new Scanner.
func NewScanner(fs fs.FS) *Scanner {
	return &Scanner{
		FS:           fs,
		SelectByName: func(_ string) bool { return true },
		Select:       func(_ string, _ os.FileInfo) bool { return true },
		Error:        func(_ string, err error) error { return err },
		Result:       func(_ string, _ ScanStats) {},
	}
}

// ScanStats collect statistics.
type ScanStats struct {
	Files, Dirs, Others uint
	Bytes               uint64
}

func (s *Scanner) scanTree(ctx context.Context, stats ScanStats, tree Tree) (ScanStats, error) {
	// traverse the path in the file system for all leaf nodes
	if tree.Leaf() {
		abstarget, err := s.FS.Abs(tree.Path)
		if err != nil {
			return ScanStats{}, err
		}

		stats, err = s.scan(ctx, stats, abstarget)
		if err != nil {
			return ScanStats{}, err
		}

		return stats, nil
	}

	// otherwise recurse into the nodes in a deterministic order
	for _, name := range tree.NodeNames() {
		var err error
		stats, err = s.scanTree(ctx, stats, tree.Nodes[name])
		if err != nil {
			return ScanStats{}, err
		}

		if ctx.Err() != nil {
			return stats, nil
		}
	}

	return stats, nil
}

// Scan traverses the targets. The function Result is called for each new item
// found, the complete result is also returned by Scan.
func (s *Scanner) Scan(ctx context.Context, targets []string) error {
	debug.Log("start scan for %v", targets)

	cleanTargets, err := resolveRelativeTargets(s.FS, targets)
	if err != nil {
		return err
	}

	debug.Log("clean targets %v", cleanTargets)

	// we're using the same tree representation as the archiver does
	tree, err := NewTree(s.FS, cleanTargets)
	if err != nil {
		return err
	}

	stats, err := s.scanTree(ctx, ScanStats{}, *tree)
	if err != nil {
		return err
	}

	s.Result("", stats)
	debug.Log("result: %+v", stats)
	return nil
}

func (s *Scanner) scan(ctx context.Context, stats ScanStats, target string) (ScanStats, error) {
	if ctx.Err() != nil {
		return stats, nil
	}

	// exclude files by path before running stat to reduce number of lstat calls
	if !s.SelectByName(target) {
		return stats, nil
	}

	// get file information
	fi, err := s.FS.Lstat(target)
	if err != nil {
		return stats, s.Error(target, err)
	}

	// run remaining select functions that require file information
	if !s.Select(target, fi) {
		return stats, nil
	}

	switch {
	case fi.Mode().IsRegular():
		stats.Files++
		stats.Bytes += uint64(fi.Size())
	case fi.Mode().IsDir():
		names, err := fs.Readdirnames(s.FS, target, fs.O_NOFOLLOW)
		if err != nil {
			return stats, s.Error(target, err)
		}
		sort.Strings(names)

		for _, name := range names {
			stats, err = s.scan(ctx, stats, filepath.Join(target, name))
			if err != nil {
				return stats, err
			}
		}
		stats.Dirs++
	default:
		stats.Others++
	}

	s.Result(target, stats)
	return stats, nil
}
