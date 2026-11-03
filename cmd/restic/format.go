package main

import (
	"fmt"
	"os"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"
)

func formatNodeOutput(n lsNodeOutput, long bool, human bool) string {
	if !long {
		return n.Path
	}

	var mode os.FileMode
	var target string

	var size string
	if human {
		size = ui.FormatBytes(n.fileSize())
	} else {
		size = fmt.Sprintf("%6d", n.fileSize())
	}

	switch n.Type {
	case data.NodeTypeFile:
		mode = 0
	case data.NodeTypeDir:
		mode = os.ModeDir
	case data.NodeTypeSymlink:
		mode = os.ModeSymlink
		target = fmt.Sprintf(" -> %v", n.LinkTarget)
	case data.NodeTypeDev:
		mode = os.ModeDevice
	case data.NodeTypeCharDev:
		mode = os.ModeDevice | os.ModeCharDevice
	case data.NodeTypeFifo:
		mode = os.ModeNamedPipe
	case data.NodeTypeSocket:
		mode = os.ModeSocket
	}

	return fmt.Sprintf("%s %5d %5d %s %s %s%s",
		mode|n.Mode, n.UID, n.GID, size,
		n.ModTime.Local().Format(global.TimeFormat), n.Path,
		target)
}

func formatNode(path string, n *data.Node, long bool, human bool) string {
	return formatNodeOutput(lsNodeOutputFrom(path, n), long, human)
}
