package restic

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/restic/restic/internal/errors"
)

// ErrNoSnapshotFound is returned when no snapshot for the given criteria could be found.
var ErrNoSnapshotFound = errors.New("no snapshot found")

// A SnapshotFilter denotes a set of snapshots based on hosts, tags and paths.
type SnapshotFilter struct {
	_ struct{} // Force naming fields in literals.

	Hosts []string
	Tags  TagLists
	Paths []string
	// Match snapshots from before this timestamp. Zero for no limit.
	TimestampLimit time.Time
}

func (f *SnapshotFilter) Empty() bool {
	return len(f.Hosts)+len(f.Tags)+len(f.Paths) == 0
}

func (f *SnapshotFilter) matches(sn *Snapshot) bool {
	return sn.HasHostname(f.Hosts) && sn.HasTagList(f.Tags) && sn.HasPaths(f.Paths)
}

// findLatest finds the latest snapshot with optional target/directory,
// tags, hostname, and timestamp filters.
func (f *SnapshotFilter) findLatest(ctx context.Context, be Lister, loader LoaderUnpacked) (*Snapshot, error) {

	var err error
	absTargets := make([]string, 0, len(f.Paths))
	for _, target := range f.Paths {
		if !filepath.IsAbs(target) {
			target, err = filepath.Abs(target)
			if err != nil {
				return nil, errors.Wrap(err, "Abs")
			}
		}
		absTargets = append(absTargets, filepath.Clean(target))
	}
	f.Paths = absTargets

	var latest *Snapshot

	err = ForAllSnapshots(ctx, be, loader, nil, func(id ID, snapshot *Snapshot, err error) error {
		if err != nil {
			return errors.Errorf("Error loading snapshot %v: %v", id.Str(), err)
		}

		if !f.TimestampLimit.IsZero() && snapshot.Time.After(f.TimestampLimit) {
			return nil
		}

		if latest != nil && snapshot.Time.Before(latest.Time) {
			return nil
		}

		if !f.matches(snapshot) {
			return nil
		}

		latest = snapshot
		return nil
	})

	if err != nil {
		return nil, err
	}

	if latest == nil {
		return nil, ErrNoSnapshotFound
	}

	return latest, nil
}

func splitSnapshotID(s string) (id, subfolder string) {
	id, subfolder, _ = strings.Cut(s, ":")
	return
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func FindSnapshot(ctx context.Context, be Lister, loader LoaderUnpacked, s string) (*Snapshot, string, error) {
	s, subfolder := splitSnapshotID(s)

	// no need to list snapshots if `s` is already a full id
	id, err := ParseID(s)
	if err != nil {
		// find snapshot id with prefix
		id, err = Find(ctx, be, SnapshotFile, s)
		if err != nil {
			return nil, "", err
		}
	}
	sn, err := LoadSnapshot(ctx, loader, id)
	return sn, subfolder, err
}

// FindLatest returns either the latest of a filtered list of all snapshots
// or a snapshot specified by `snapshotID`.
func (f *SnapshotFilter) FindLatest(ctx context.Context, be Lister, loader LoaderUnpacked, snapshotID string) (*Snapshot, string, error) {
	id, subfolder := splitSnapshotID(snapshotID)
	if id == "latest" {
		sn, err := f.findLatest(ctx, be, loader)
		if err == ErrNoSnapshotFound {
			err = fmt.Errorf("snapshot filter (Paths:%v Tags:%v Hosts:%v): %w",
				f.Paths, f.Tags, f.Hosts, err)
		}
		return sn, subfolder, err
	}
	return FindSnapshot(ctx, be, loader, snapshotID)
}

type SnapshotFindCb func(string, *Snapshot, error) error

var ErrInvalidSnapshotSyntax = errors.New("<snapshot>:<subfolder> syntax not allowed")

// FindAll yields Snapshots, either given explicitly by `snapshotIDs` or filtered from the list of all snapshots.
func (f *SnapshotFilter) FindAll(ctx context.Context, be Lister, loader LoaderUnpacked, snapshotIDs []string, fn SnapshotFindCb) error {
	if len(snapshotIDs) != 0 {
		var err error
		usedFilter := false

		ids := NewIDSet()
		// Process all snapshot IDs given as arguments.
		for _, s := range snapshotIDs {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			var sn *Snapshot
			if s == "latest" {
				if usedFilter {
					continue
				}

				usedFilter = true

				sn, err = f.findLatest(ctx, be, loader)
				if err == ErrNoSnapshotFound {
					err = errors.Errorf("no snapshot matched given filter (Paths:%v Tags:%v Hosts:%v)",
						f.Paths, f.Tags, f.Hosts)
				}
				if sn != nil {
					ids.Insert(*sn.ID())
				}
			} else if strings.HasPrefix(s, "latest:") {
				err = ErrInvalidSnapshotSyntax
			} else {
				var subfolder string
				sn, subfolder, err = FindSnapshot(ctx, be, loader, s)
				if err == nil && subfolder != "" {
					err = ErrInvalidSnapshotSyntax
				} else if err == nil {
					if ids.Has(*sn.ID()) {
						continue
					}

					ids.Insert(*sn.ID())
					s = sn.ID().String()
				}
			}
			err = fn(s, sn, err)
			if err != nil {
				return err
			}
		}

		// Give the user some indication their filters are not used.
		if !usedFilter && !f.Empty() {
			return fn("filters", nil, errors.Errorf("explicit snapshot ids are given"))
		}
		return nil
	}

	return ForAllSnapshots(ctx, be, loader, nil, func(id ID, sn *Snapshot, err error) error {
		if err == nil && !f.matches(sn) {
			return nil
		}

		return fn(id.String(), sn, err)
	})
}
