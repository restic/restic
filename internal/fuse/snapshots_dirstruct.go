//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

type MetaDirData struct {
	// set if this is a symlink or a snapshot mount point
	linkTarget string
	snapshot   *restic.Snapshot
	// names is set if this is a pseudo directory
	names map[string]*MetaDirData
}

// SnapshotsDirStructure contains the directory structure for snapshots.
// It uses a paths and time template to generate a map of pathnames
// pointing to the actual snapshots. For templates that end with a time,
// also "latest" links are generated.
type SnapshotsDirStructure struct {
	root          *Root
	pathTemplates []string
	timeTemplate  string

	mutex sync.Mutex
	// "" is the root path, subdirectory paths are assembled as parent+"/"+childFn
	// thus all subdirectories are prefixed with a slash as the root is ""
	// that way we don't need path processing special cases when using the entries tree
	entries map[string]*MetaDirData

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
			if len(sn.Tags) == 0 {
				return nil, ""
			}
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

		default:
			repl = string(c)
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

// determine static path prefix
func staticPrefix(pathTemplate string) (prefix string) {
	inVerb := false
	patternStart := -1
outer:
	for i, c := range pathTemplate {
		if !inVerb {
			if c == '%' {
				inVerb = true
			}
			continue
		}
		inVerb = false
		switch c {
		case 'i', 'I', 'u', 'h', 't', 'T':
			patternStart = i
			break outer
		}
	}
	if patternStart < 0 {
		// ignore patterns without template variable
		return ""
	}

	p := pathTemplate[:patternStart]
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return ""
	}
	return p[:idx]
}

// uniqueName returns a unique name to be used for prefix+name.
// It appends -number to make the name unique.
func uniqueName(entries map[string]*MetaDirData, prefix, name string) string {
	newname := name
	for i := 1; ; i++ {
		if _, ok := entries[prefix+newname]; !ok {
			break
		}
		newname = fmt.Sprintf("%s-%d", name, i)
	}
	return newname
}

// makeDirs inserts all paths generated from pathTemplates and
// TimeTemplate for all given snapshots into d.names.
// Also adds d.latest links if "%T" is at end of a path template
func (d *SnapshotsDirStructure) makeDirs(snapshots restic.Snapshots) {
	entries := make(map[string]*MetaDirData)

	type mountData struct {
		sn         *restic.Snapshot
		linkTarget string // if linkTarget!= "", this is a symlink
		childFn    string
		child      *MetaDirData
	}

	// recursively build tree structure
	var mount func(path string, data mountData)
	mount = func(path string, data mountData) {
		e := entries[path]
		if e == nil {
			e = &MetaDirData{}
		}
		if data.sn != nil {
			e.snapshot = data.sn
			e.linkTarget = data.linkTarget
		} else {
			// intermediate directory, register as a child directory
			if e.names == nil {
				e.names = make(map[string]*MetaDirData)
			}
			if data.child != nil {
				e.names[data.childFn] = data.child
			}
		}
		entries[path] = e

		slashIdx := strings.LastIndex(path, "/")
		if slashIdx >= 0 {
			// add to parent dir, but without snapshot
			mount(path[:slashIdx], mountData{childFn: path[slashIdx+1:], child: e})
		}
	}

	// root directory
	mount("", mountData{})

	// insert pure directories; needed to get empty structure even if there
	// are no snapshots in these dirs
	for _, p := range d.pathTemplates {
		p = staticPrefix(p)
		if p != "" {
			mount(path.Clean("/"+p), mountData{})
		}
	}

	latestTime := make(map[string]time.Time)
	for _, sn := range snapshots {
		for _, templ := range d.pathTemplates {
			paths, timeSuffix := pathsFromSn(templ, d.timeTemplate, sn)
			for _, p := range paths {
				if p != "" {
					p = "/" + p
				}
				suffix := uniqueName(entries, p, timeSuffix)
				mount(path.Clean(p+suffix), mountData{sn: sn})
				if timeSuffix != "" {
					lt, ok := latestTime[p]
					if !ok || !sn.Time.Before(lt) {
						debug.Log("link (update) %v -> %v\n", p, suffix)
						// inject symlink
						mount(path.Clean(p+"/latest"), mountData{sn: sn, linkTarget: suffix})
						latestTime[p] = sn.Time
					}
				}
			}
		}
	}

	d.entries = entries
}

const minSnapshotsReloadTime = 60 * time.Second

// update snapshots if repository has changed
func (d *SnapshotsDirStructure) updateSnapshots(ctx context.Context) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
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

func (d *SnapshotsDirStructure) UpdatePrefix(ctx context.Context, prefix string) (*MetaDirData, error) {
	err := d.updateSnapshots(ctx)
	if err != nil {
		return nil, err
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.entries[prefix], nil
}
