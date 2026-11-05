package fuse

import "github.com/restic/restic/internal/data"

// Config holds settings for the fuse mount.
type Config struct {
	OwnerIsRoot   bool
	Filter        data.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

var defaultPathTemplates = []string{
	"ids/%i",
	"snapshots/%T",
	"hosts/%h/%T",
	"tags/%t/%T",
}

// Size of the blob cache. TODO: make this configurable.
const blobCacheSize = 64 << 20
