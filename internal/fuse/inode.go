//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"encoding/binary"

	"github.com/cespare/xxhash/v2"
	"github.com/restic/restic/internal/restic"
)

// inodeFromName generates an inode number for a file in a meta dir.
func inodeFromName(parent uint64, name string) uint64 {
	inode := parent ^ xxhash.Sum64String(cleanupNodeName(name))

	// Inode 0 is invalid and 1 is the root. Remap those.
	if inode < 2 {
		inode += 2
	}
	return inode
}

// inodeFromNode generates an inode number for a file within a snapshot.
func inodeFromNode(parent uint64, node *restic.Node) (inode uint64) {
	if node.Links > 1 && node.Type != "dir" {
		// If node has hard links, give them all the same inode,
		// irrespective of the parent.
		var buf [16]byte
		binary.LittleEndian.PutUint64(buf[:8], node.DeviceID)
		binary.LittleEndian.PutUint64(buf[8:], node.Inode)
		inode = xxhash.Sum64(buf[:])
	} else {
		// Else, use the name and the parent inode.
		// node.{DeviceID,Inode} may not even be reliable.
		inode = parent ^ xxhash.Sum64String(cleanupNodeName(node.Name))
	}

	// Inode 0 is invalid and 1 is the root. Remap those.
	if inode < 2 {
		inode += 2
	}
	return inode
}
