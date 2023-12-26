package main

import (
	"fmt"
	"os"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
)

func formatNode(path string, n *restic.Node, long bool, human bool) string {
	if !long {
		return path
	}

	var mode os.FileMode
	var target string

	var size string
	if human {
		size = ui.FormatBytes(n.Size)
	} else {
		size = fmt.Sprintf("%6d", n.Size)
	}

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

	return fmt.Sprintf("%s %5d %5d %s %s %s%s",
		mode|n.Mode, n.UID, n.GID, size,
		n.ModTime.Local().Format(TimeFormat), path,
		target)
}
