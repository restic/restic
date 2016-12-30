package backend

import (
	"os"
	"path"
	"restic"
)

// Paths contains the default paths for file-based backends (e.g. local).
var Paths = struct {
	Data      string
	Snapshots string
	Index     string
	Locks     string
	Keys      string
	Temp      string
	Config    string
}{
	"data",
	"snapshots",
	"index",
	"locks",
	"keys",
	"tmp",
	"config",
}

// Modes holds the default modes for directories and files for file-based
// backends.
var Modes = struct{ Dir, File os.FileMode }{0700, 0600}

// Join joins the given paths and cleans them afterwards. This always uses
// forward slashes, which is required by all non local backends.
func Join(parts ...string) string {
	return path.Clean(path.Join(parts...))
}

// Filename constructs the path for given base name restic.Type and
// name.
//
// This can optionally use a number of characters of the blob name as
// a directory prefix to partition blobs into smaller directories.
//
// Currently local and sftp repositories are handling blob filenames
// using a dirPrefixLen of 2.
func Filename(base string, t restic.FileType, name string, dirPrefixLen int) string {
	if t == restic.ConfigFile {
		return Join(base, Paths.Config)
	}

	return Join(base, Dirname(t, name, dirPrefixLen), name)
}

// Dirname constructs the directory name for a given FileType and file
// name using the path names defined in Paths.
//
// This can optionally use a number of characters of the blob name as
// a directory prefix to partition blobs into smaller directories.
//
// Currently local and sftp repositories are handling blob filenames
// using a dirPrefixLen of 2.
func Dirname(t restic.FileType, name string, dirPrefixLen int) string {
	var n string
	switch t {
	case restic.DataFile:
		n = Paths.Data
		if dirPrefixLen > 0 && len(name) > dirPrefixLen {
			n = path.Join(n, name[:dirPrefixLen])
		}
	case restic.SnapshotFile:
		n = Paths.Snapshots
	case restic.IndexFile:
		n = Paths.Index
	case restic.LockFile:
		n = Paths.Locks
	case restic.KeyFile:
		n = Paths.Keys
	}
	return n
}
