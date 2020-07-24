// +build darwin freebsd linux windows

package fuse

import "github.com/restic/restic/internal/restic"

// Config holds settings for the fuse mount.
type Config struct {
	OwnerIsRoot      bool
	Hosts            []string
	Tags             []restic.TagList
	Paths            []string
	SnapshotTemplate string
}
