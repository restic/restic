package restic

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/restic/restic/internal/errors"
)

// SnapshotGroupKey is the structure for identifying groups in a grouped
// snapshot list. This is used by GroupSnapshots()
type SnapshotGroupKey struct {
	Hostname string   `json:"hostname"`
	Paths    []string `json:"paths"`
	Tags     []string `json:"tags"`
}

// GroupSnapshots takes a list of snapshots and a grouping criteria and creates
// a group list of snapshots.
func GroupSnapshots(snapshots Snapshots, options string) (map[string]Snapshots, bool, error) {
	// group by hostname and dirs
	snapshotGroups := make(map[string]Snapshots)

	var GroupByTag bool
	var GroupByHost bool
	var GroupByPath bool
	var GroupOptionList []string

	GroupOptionList = strings.Split(options, ",")

	for _, option := range GroupOptionList {
		switch option {
		case "host", "hosts":
			GroupByHost = true
		case "path", "paths":
			GroupByPath = true
		case "tag", "tags":
			GroupByTag = true
		case "":
		default:
			return nil, false, errors.Fatal("unknown grouping option: '" + option + "'")
		}
	}

	for _, sn := range snapshots {
		// Determining grouping-keys
		var tags []string
		var hostname string
		var paths []string

		if GroupByTag {
			tags = sn.Tags
			sort.StringSlice(tags).Sort()
		}
		if GroupByHost {
			hostname = sn.Hostname
		}
		if GroupByPath {
			paths = sn.Paths
		}

		sort.StringSlice(sn.Paths).Sort()
		var k []byte
		var err error

		k, err = json.Marshal(SnapshotGroupKey{Tags: tags, Hostname: hostname, Paths: paths})

		if err != nil {
			return nil, false, err
		}
		snapshotGroups[string(k)] = append(snapshotGroups[string(k)], sn)
	}

	return snapshotGroups, GroupByTag || GroupByHost || GroupByPath, nil
}
