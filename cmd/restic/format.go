package main

import (
	"fmt"
	"os"

	"github.com/restic/restic/internal/restic"
)

func formatNode(path string, n *restic.Node, long bool) string {
	if !long {
		return path
	}

	var mode os.FileMode
	var target string

	switch n.Type {
	case "file":
		mode = 0
	case "dir":
		mode = os.ModeDir
	case "symlink":
		mode = os.ModeSymlink
		target = fmt.Sprintf(" -> %v", n.LinkTarget)
	case "dev":
		mode = os.ModeDevice
	case "chardev":
		mode = os.ModeDevice | os.ModeCharDevice
	case "fifo":
		mode = os.ModeNamedPipe
	case "socket":
		mode = os.ModeSocket
	}

	return fmt.Sprintf("%s %5d %5d %6d %s %s%s",
		mode|n.Mode, n.UID, n.GID, n.Size,
		n.ModTime.Local().Format(TimeFormat), path,
		target)
}
