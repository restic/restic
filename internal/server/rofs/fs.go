// Package rofs implements a read-only file system on top of a restic repository.
package rofs

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// ROFS implements a read-only filesystem on top of a repo.
type ROFS struct {
	repo     restic.Repository
	cfg      Config
	entries  map[string]rofsEntry
	fileInfo fileInfo
}

type rofsEntry interface {
	Open() (fs.File, error)
	DirEntry() fs.DirEntry
}

// statically ensure that *FS implements fs.FS
var _ fs.FS = &ROFS{}

// Config holds settings for a filesystem.
type Config struct {
	Filter        restic.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

// New returns a new filesystem for the repo.
func New(ctx context.Context, repo restic.Repository, cfg Config) (*ROFS, error) {
	// set defaults, if PathTemplates is not set
	if len(cfg.PathTemplates) == 0 {
		cfg.PathTemplates = []string{
			"ids/%i",
			"snapshots/%T",
			"hosts/%h/%T",
			"tags/%t/%T",
		}
	}

	rofs := &ROFS{
		repo:    repo,
		cfg:     cfg,
		entries: make(map[string]rofsEntry),
		fileInfo: fileInfo{
			name:    ".",
			mode:    0755,
			modtime: time.Now(),
		},
	}

	err := rofs.updateSnapshots(ctx)
	if err != nil {
		return nil, err
	}

	return rofs, nil
}

func (rofs *ROFS) updateSnapshots(ctx context.Context) error {

	entries, err := buildSnapshotEntries(ctx, rofs.repo, rofs.cfg)
	if err != nil {
		return err
	}

	rofs.entries = entries

	return nil
}

func buildSnapshotEntries(ctx context.Context, repo restic.Repository, cfg Config) (map[string]rofsEntry, error) {
	var snapshots restic.Snapshots
	err := cfg.Filter.FindAll(ctx, repo, repo, nil, func(_ string, sn *restic.Snapshot, _ error) error {
		if sn != nil {
			snapshots = append(snapshots, sn)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("filter snapshots: %w", err)
	}

	debug.Log("found %d snapshots", len(snapshots))

	list := make(map[string]rofsEntry)
	list["foo"] = NewMemFile("foo", []byte("foobar content of file foo"), time.Now())

	list["snapshots"] = NewSnapshotsDir(ctx, repo, cfg.PathTemplates, cfg.TimeTemplate)

	return list, nil
}

// Open opens the named file.
//
// When Open returns an error, it should be of type *PathError
// with the Op field set to "open", the Path field set to name,
// and the Err field describing the problem.
//
// Open should reject attempts to open names that do not satisfy
// ValidPath(name), returning a *PathError with Err set to
// ErrInvalid or ErrNotExist.
func (rofs *ROFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		debug.Log("Open(%v), invalid path name", name)

		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}

	if name == "." {
		debug.Log("Open(%v) (root)", name)

		d := &openDir{
			path: ".",
			fileInfo: fileInfo{
				name:    ".",
				mode:    fs.ModeDir | 0555,
				modtime: time.Now(),
			},
			entries: dirMap2DirEntry(rofs.entries),
		}

		return d, nil
	}

	entry, ok := rofs.entries[name]
	if !ok {
		debug.Log("Open(%v) -> does not exist", name)
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}

	debug.Log("Open(%v)", name)

	return entry.Open()
}
