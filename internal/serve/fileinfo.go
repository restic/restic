package serve

import (
	"os"
	"time"

	"github.com/restic/restic/internal/restic"
)

// virtFileInfo is used to construct an os.FileInfo for a server.
type virtFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modtime time.Time
	isdir   bool
}

// statically ensure that virtFileInfo implements os.FileInfo.
var _ os.FileInfo = virtFileInfo{}

func (fi virtFileInfo) Name() string       { return fi.name }
func (fi virtFileInfo) Size() int64        { return fi.size }
func (fi virtFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi virtFileInfo) ModTime() time.Time { return fi.modtime }
func (fi virtFileInfo) IsDir() bool        { return fi.isdir }
func (fi virtFileInfo) Sys() interface{}   { return nil }

func fileInfoFromNode(node *restic.Node) os.FileInfo {
	fi := virtFileInfo{
		name:    node.Name,
		size:    int64(node.Size),
		mode:    node.Mode,
		modtime: node.ModTime,
	}

	if node.Type == "dir" {
		fi.isdir = true
		fi.mode |= os.ModeDir
	}

	return fi
}
