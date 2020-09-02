//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// SnapshotsDirStructure contains the directory structure for snapshots.
// It uses a paths and time template to generate a map of pathnames
// pointing to the actual snapshots. For templates that end with a time,
// also "latest" links are generated.
type SnapshotsDirStructure struct {
	root          *Root
	pathTemplates []string
	timeTemplate  string

	names     map[string]*restic.Snapshot
	latest    map[string]string
	snCount   int
	lastCheck time.Time
}

// NewSnapshotsDirStructure returns a new directory structure for snapshots.
func NewSnapshotsDirStructure(root *Root, pathTemplates []string, timeTemplate string) *SnapshotsDirStructure {
	return &SnapshotsDirStructure{
		root:          root,
		pathTemplates: pathTemplates,
		timeTemplate:  timeTemplate,
		snCount:       -1,
	}
}

// uniqueName returns a unique name to be used for prefix+name.
// It appends -number to make the name unique.
func (d *SnapshotsDirStructure) uniqueName(prefix, name string) (newname string) {
	newname = name
	for i := 1; ; i++ {
		if _, ok := d.names[prefix+newname]; !ok {
			break
		}
		newname = fmt.Sprintf("%s-%d", name, i)
	}
	return newname
}

// pathsFromSn generates the paths from pathTemplate and timeTemplate
// where the variables are replaced by the snapshot data.
// The time is given as suffix if the pathTemplate ends with "%T".
func pathsFromSn(pathTemplate string, timeTemplate string, sn *restic.Snapshot) (paths []string, timeSuffix string) {
	timeformat := sn.Time.Format(timeTemplate)

	inVerb := false
	writeTime := false
	out := make([]strings.Builder, 1)
	for _, c := range pathTemplate {
		if writeTime {
			for i := range out {
				out[i].WriteString(timeformat)
			}
			writeTime = false
		}

		if !inVerb {
			if c == '%' {
				inVerb = true
			} else {
				for i := range out {
					out[i].WriteRune(c)
				}
			}
			continue
		}

		var repl string
		inVerb = false
		switch c {
		case 'T':
			// lazy write; time might be returned as suffix
			writeTime = true
			continue

		case 't':
			if len(sn.Tags) != 1 {
				// needs special treatment: Rebuild the string builders
				newout := make([]strings.Builder, len(out)*len(sn.Tags))
				for i, tag := range sn.Tags {
					for j := range out {
						newout[i*len(out)+j].WriteString(out[j].String() + tag)
					}
				}
				out = newout
				continue
			}
			repl = sn.Tags[0]

		case 'i':
			repl = sn.ID().Str()

		case 'I':
			repl = sn.ID().String()

		case 'u':
			repl = sn.Username

		case 'h':
			repl = sn.Hostname
		}

		// write replacement string to all string builders
		for i := range out {
			out[i].WriteString(repl)
		}
	}

	for i := range out {
		paths = append(paths, out[i].String())
	}

	if writeTime {
		timeSuffix = timeformat
	}

	return paths, timeSuffix
}

// makeDirs inserts all paths generated from pathTemplates and
// TimeTemplate for all given snapshots into d.names.
// Also adds d.latest links if "%T" is at end of a path template
func (d *SnapshotsDirStructure) makeDirs(snapshots restic.Snapshots) {
	d.names = make(map[string]*restic.Snapshot)
	d.latest = make(map[string]string)

	// insert pure directories; needed to get empty structure even if there
	// are no snapshots in these dirs
	for _, p := range d.pathTemplates {
		for _, pattern := range []string{"%i", "%I", "%u", "%h", "%t", "%T"} {
			p = strings.ReplaceAll(p, pattern, "")
		}
		d.names[path.Clean(p)+"/"] = nil
	}

	latestTime := make(map[string]time.Time)
	for _, sn := range snapshots {
		for _, templ := range d.pathTemplates {
			paths, timeSuffix := pathsFromSn(templ, d.timeTemplate, sn)
			for _, p := range paths {
				suffix := d.uniqueName(p, timeSuffix)
				d.names[path.Clean(p+suffix)] = sn
				if timeSuffix != "" {
					lt, ok := latestTime[p]
					if !ok || !sn.Time.Before(lt) {
						debug.Log("link (update) %v -> %v\n", p, suffix)
						d.latest[p] = suffix
						latestTime[p] = sn.Time
					}
				}
			}
		}
	}
}

const minSnapshotsReloadTime = 60 * time.Second

// update snapshots if repository has changed
func (d *SnapshotsDirStructure) updateSnapshots(ctx context.Context) error {
	if time.Since(d.lastCheck) < minSnapshotsReloadTime {
		return nil
	}

	snapshots, err := restic.FindFilteredSnapshots(ctx, d.root.repo.Backend(), d.root.repo, d.root.cfg.Hosts, d.root.cfg.Tags, d.root.cfg.Paths)
	if err != nil {
		return err
	}
	// sort snapshots ascending by time (default order is descending)
	sort.Sort(sort.Reverse(snapshots))

	d.lastCheck = time.Now()

	if d.snCount == len(snapshots) {
		return nil
	}

	err = d.root.repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	d.snCount = len(snapshots)

	d.makeDirs(snapshots)
	return nil
}
