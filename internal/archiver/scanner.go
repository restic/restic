package archiver

import (
	"context"
	"os"
	"path/filepath"

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
		SelectByName: func(item string) bool { return true },
		Select:       func(item string, fi os.FileInfo) bool { return true },
		Error:        func(item string, fi os.FileInfo, err error) error { return err },
		Result:       func(item string, s ScanStats) {},
	}
}

// ScanStats collect statistics.
type ScanStats struct {
	Files, Dirs, Others uint
	Bytes               uint64
}

// Scan traverses the targets. The function Result is called for each new item
// found, the complete result is also returned by Scan.
func (s *Scanner) Scan(ctx context.Context, targets []string) error {
	var stats ScanStats
	for _, target := range targets {
		abstarget, err := s.FS.Abs(target)
		if err != nil {
			return err
		}

		stats, err = s.scan(ctx, stats, abstarget)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return nil
		}
	}

	s.Result("", stats)
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
		return stats, s.Error(target, fi, err)
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
		names, err := readdirnames(s.FS, target)
		if err != nil {
			return stats, s.Error(target, fi, err)
		}

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
