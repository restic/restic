package restic

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/restic/restic/internal/errors"
)

type SnapshotGroupByOptions struct {
	Tag  bool
	Host bool
	Path bool
}

func splitSnapshotGroupBy(s string) (SnapshotGroupByOptions, error) {
	var l SnapshotGroupByOptions
	for _, option := range strings.Split(s, ",") {
		switch option {
		case "host", "hosts":
			l.Host = true
		case "path", "paths":
			l.Path = true
		case "tag", "tags":
			l.Tag = true
		case "":
		default:
			return SnapshotGroupByOptions{}, errors.Fatal("unknown grouping option: '" + option + "'")
		}
	}
	return l, nil
}

func (l SnapshotGroupByOptions) String() string {
	var parts []string
	if l.Host {
		parts = append(parts, "host")
	}
	if l.Path {
		parts = append(parts, "paths")
	}
	if l.Tag {
		parts = append(parts, "tags")
	}
	return strings.Join(parts, ",")
}

func (l *SnapshotGroupByOptions) Set(s string) error {
	parts, err := splitSnapshotGroupBy(s)
	if err != nil {
		return err
	}
	*l = parts
	return nil
}

func (l *SnapshotGroupByOptions) Type() string {
	return "group"
}

// SnapshotGroupKey is the structure for identifying groups in a grouped
// snapshot list. This is used by GroupSnapshots()
type SnapshotGroupKey struct {
	Hostname string   `json:"hostname"`
	Paths    []string `json:"paths"`
	Tags     []string `json:"tags"`
}

// GroupSnapshots takes a list of snapshots and a grouping criteria and creates
// a group list of snapshots.
func GroupSnapshots(snapshots Snapshots, groupBy SnapshotGroupByOptions) (map[string]Snapshots, bool, error) {
	// group by hostname and dirs
	snapshotGroups := make(map[string]Snapshots)

	for _, sn := range snapshots {
		// Determining grouping-keys
		var tags []string
		var hostname string
		var paths []string

		if groupBy.Tag {
			tags = sn.Tags
			sort.Strings(tags)
		}
		if groupBy.Host {
			hostname = sn.Hostname
		}
		if groupBy.Path {
			paths = sn.Paths
		}

		sort.Strings(sn.Paths)
		var k []byte
		var err error

		k, err = json.Marshal(SnapshotGroupKey{Tags: tags, Hostname: hostname, Paths: paths})

		if err != nil {
			return nil, false, err
		}
		snapshotGroups[string(k)] = append(snapshotGroups[string(k)], sn)
	}

	return snapshotGroups, groupBy.Tag || groupBy.Host || groupBy.Path, nil
}
