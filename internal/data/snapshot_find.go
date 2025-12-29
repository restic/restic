package data

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
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

	// these DurationTime refer to --older-than, --newer-than and --relative-to
	OlderThan  DurationTime
	NewerThan  DurationTime
	RelativeTo DurationTime
}

func (f *SnapshotFilter) Empty() bool {
	return len(f.Hosts)+len(f.Tags)+len(f.Paths) == 0 && f.NewerThan.Empty() && f.OlderThan.Empty()
}

func (f *SnapshotFilter) matches(sn *Snapshot) bool {
	if !sn.HasHostname(f.Hosts) || !sn.HasTagList(f.Tags) || !sn.HasPaths(f.Paths) {
		return false
	}

	// time checking; `--newer-than` <= snapshotTime && snapshotTime <= `--older-than`
	testOlderThan := true
	testNewerThan := true
	if f.NewerThan.state == durationTimeSet { // need "<="  which is "! >"
		testNewerThan = !f.NewerThan.GetTime().After(sn.Time)
	}
	if f.OlderThan.state == durationTimeSet {
		testOlderThan = !sn.Time.After(f.OlderThan.GetTime())
	}
	return testOlderThan && testNewerThan
}

// findLatest finds the latest snapshot with optional target/directory,
// tags, hostname, and timestamp filters.
func (f *SnapshotFilter) findLatest(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked) (*Snapshot, error) {

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

	err = ForAllSnapshots(ctx, be, loader, nil, func(id restic.ID, snapshot *Snapshot, err error) error {
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
func FindSnapshot(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked, s string) (*Snapshot, string, error) {
	s, subfolder := splitSnapshotID(s)

	// no need to list snapshots if `s` is already a full id
	id, err := restic.ParseID(s)
	if err != nil {
		// find snapshot id with prefix
		id, err = restic.Find(ctx, be, restic.SnapshotFile, s)
		if err != nil {
			return nil, "", err
		}
	}
	sn, err := LoadSnapshot(ctx, loader, id)
	return sn, subfolder, err
}

// FindLatest returns either the latest of a filtered list of all snapshots
// or a snapshot specified by `snapshotID`.
func (f *SnapshotFilter) FindLatest(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked, snapshotID string) (*Snapshot, string, error) {
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
func (f *SnapshotFilter) FindAll(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked, snapshotIDs []string, fn SnapshotFindCb) error {
	// called once to resolve snapIDs and other use cases
	err := f.buildSnapTimes(ctx, be, loader)
	if err != nil {
		return err
	}

	if len(snapshotIDs) != 0 {
		var err error
		usedFilter := false

		ids := restic.NewIDSet()
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

	return ForAllSnapshots(ctx, be, loader, nil, func(id restic.ID, sn *Snapshot, err error) error {
		if err == nil && !f.matches(sn) {
			return nil
		}

		return fn(id.String(), sn, err)
	})
}

// setTimeFilters is called once to evaluate the 'relative' times into absolute
// times. snapIDs are converted to their sn.Time, and restic.durations are
// calculated as f.RelativeTo.timeReference - restic.duration, see setTimes() below
func (f *SnapshotFilter) setTimeFilters(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked) error {

	// if durationTypes are requested,
	if (f.NewerThan.state == durationType || f.OlderThan.state == durationType) && f.RelativeTo.state == durationUninitialized {
		f.RelativeTo.snapID = "latest"
		f.RelativeTo.state = durationSnapID
	}

	needSnapIDs := make([]string, 0, 3)
	memory := make(map[string]*Snapshot)
	durationsNeeded := make([]*DurationTime, 0, 3)
	for _, reference := range []*DurationTime{&f.RelativeTo, &f.OlderThan, &f.NewerThan} {
		if reference.state == durationSnapID {
			needSnapIDs = append(needSnapIDs, reference.snapID)
			durationsNeeded = append(durationsNeeded, reference)
		}
	}

	for i, snapID := range needSnapIDs {
		var sn *Snapshot
		var err error
		if snTemp, ok := memory[snapID]; ok {
			sn = snTemp
		} else if snapID == "latest" {
			sn, err = f.findLatest(ctx, be, loader)
			if err != nil {
				return err
			}
			memory[(*sn).ID().Str()] = sn
			memory[snapID] = sn
		} else {
			sn, _, err = FindSnapshot(ctx, be, loader, snapID)
			if err != nil {
				return err
			}
			memory[snapID] = sn
		}
		// the .Local() is critical, it treats this time as a  local time
		(*durationsNeeded[i]).timeReference = (*sn).Time.Local()
		(*durationsNeeded[i]).state = durationTimeSet
	}

	return nil
}

// buildSnapTimes checks if snapID or 'latest' are used in time based filters.
// If that is so, 'setTimeFilters()' gathers all these snapshots and converts
// them to time.Time entries using snapshot.Time
// snapshot 'latest' is needed for Duration based offsets, when no '--relative-to'
// is given.
func (f *SnapshotFilter) buildSnapTimes(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked) error {
	if f.RelativeTo.state == durationSnapID || f.NewerThan.state == durationSnapID || f.OlderThan.state == durationSnapID ||
		f.NewerThan.state == durationType || f.OlderThan.state == durationType {

		err := f.setTimeFilters(ctx, be, loader)
		if err != nil {
			return err
		}
	}

	return f.setTimes()
}

// setTimes converts a restic.Duration into a time.Time with the offset
// defined in Duration. In addition setTimes does some health checks
func (f *SnapshotFilter) setTimes() error {
	switch f.RelativeTo.state {
	case durationUninitialized, durationTimeSet:
		// do nothing, fall through
	case durationType, durationSnapID:
		return errors.Fatalf("a valid --relative-to can only be a time value - should never happen, but it is a %v", f.RelativeTo)
	}

	switch f.OlderThan.state {
	case durationUninitialized, durationTimeSet:
	case durationType:
		f.OlderThan = f.RelativeTo.AddOffset(f.OlderThan)
	case durationSnapID:
		panic(fmt.Sprintf("internal error: OlderThan = %s", f.OlderThan.String()))
	}

	switch f.NewerThan.state {
	case durationUninitialized, durationTimeSet:
	case durationType:
		f.NewerThan = f.RelativeTo.AddOffset(f.NewerThan)
	case durationSnapID:
		panic(fmt.Sprintf("internal error: OlderThan = %s", f.NewerThan.String()))
	}

	// check `--newer-than` <= `--older-than`
	if f.NewerThan.state == durationTimeSet && f.OlderThan.state == durationTimeSet && f.NewerThan.GetTime().After(f.OlderThan.GetTime()) {
		return errors.Fatalf("invalid time comparison times: '--newer-than (%s)' should be <= '--older-than (%s)'"+
			"\ntry reversing --older-than and --newer-than",
			f.NewerThan.GetTime().Format(time.DateTime), f.OlderThan.GetTime().Format(time.DateTime))
	}

	if f.OlderThan.state != durationUninitialized {
		debug.Log("filter OlderThan  %s", f.OlderThan.String())
	}
	if f.NewerThan.state != durationUninitialized {
		debug.Log("filter NewerThan  %s", f.NewerThan.String())
	}
	if f.RelativeTo.state != durationUninitialized {
		debug.Log("filter RelativeTo %s", f.RelativeTo.String())
	}
	if len(f.Hosts) > 0 {
		debug.Log("filter Hosts %v", f.Hosts)
	}
	if len(f.Paths) > 0 {
		debug.Log("filter Paths %v", f.Paths)
	}

	return nil
}
