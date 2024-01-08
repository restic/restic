package frontend

import (
	"os"
	"path"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

type LocalFileMetadata struct {
	frontend *LocalFrontend
	fi       os.FileInfo
	path     string
}

// statically ensure that GoogleDriveFileMetadata implements FileMetadata.
var _ restic.FileMetadata = &LocalFileMetadata{}

func (fm *LocalFileMetadata) Equal(other restic.FileMetadata) bool {
	if other == nil {
		return false
	}
	return fm.path == other.Path()
}

func (fm *LocalFileMetadata) DeviceID() (deviceID uint64, err error) {
	return fs.DeviceID(fm.fi)
}

func (fm *LocalFileMetadata) Size() int64 {
	return fm.fi.Size()
}

func (fm *LocalFileMetadata) Path() string {
	return fm.path
}

func (fm *LocalFileMetadata) AbsPath() (string, error) {
	return fm.frontend.FS.Abs(fm.path)
}

func (fm *LocalFileMetadata) Children() ([]restic.LazyFileMetadata, error) {
	return fm.children(fs.O_NOFOLLOW)
}

func (fm *LocalFileMetadata) ChildrenWithFlag(flag int) ([]restic.LazyFileMetadata, error) {
	return fm.children(flag)
}

func (fm *LocalFileMetadata) children(flags int) ([]restic.LazyFileMetadata, error) {
	names, err := fm.frontend.readdirnames(fm.path, flags)
	if err != nil {
		return nil, err
	}
	result := make([]restic.LazyFileMetadata, len(names))
	for i, name := range names {
		result[i] = &LocalLazyFileMetadata{
			frontend: fm.frontend,
			name:     name,
			path:     fm.frontend.FS.Join(fm.path, name),
		}
	}
	return result, nil
}

// nodeFromFileInfo returns the restic node from an os.FileInfo.
func (fm *LocalFileMetadata) Node(snPath string, withAtime bool) (*restic.Node, error) {
	node, err := restic.NodeFromFileInfo(fm.path, fm.fi)
	if !withAtime {
		node.AccessTime = node.ModTime
	}
	// overwrite name to match that within the snapshot
	node.Name = path.Base(snPath)
	return node, errors.WithStack(err)
}

// fileChanged tries to detect whether a file's content has changed compared
// to the contents of node, which describes the same path in the parent backup.
// It should only be run for regular files.
func (fm *LocalFileMetadata) FileChanged(node *restic.Node) bool {
	switch {
	case node == nil:
		return true
	case node.Type != "file":
		// We're only called for regular files, so this is a type change.
		return true
	case uint64(fm.fi.Size()) != node.Size:
		return true
	case !fm.fi.ModTime().Equal(node.ModTime):
		return true
	}

	checkCtime := fm.frontend.ChangeIgnoreFlags&ChangeIgnoreCtime == 0
	checkInode := fm.frontend.ChangeIgnoreFlags&ChangeIgnoreInode == 0

	extFI := fs.ExtendedStat(fm.fi)
	switch {
	case checkCtime && !extFI.ChangeTime.Equal(node.ChangeTime):
		return true
	case checkInode && node.Inode != extFI.Inode:
		return true
	}

	return false
}

func (fm *LocalFileMetadata) Mode() restic.FileMode {
	switch {
	case fs.IsRegularFile(fm.fi):
		return restic.REGULAR
	case fm.fi.IsDir():
		return restic.DIRECTORY
	case fm.fi.Mode()&os.ModeSocket > 0:
		return restic.SOCKET
	default:
		return restic.OTHER
	}
}

func (fm *LocalFileMetadata) Name() string {
	return fm.fi.Name()
}
