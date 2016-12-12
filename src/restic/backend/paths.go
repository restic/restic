package backend

import (
	"os"
	"path/filepath"

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

// Dirname constructs the default directory for given FileType.
func Dirname(base string, t restic.FileType, name string) string {
	var n string
	switch t {
	case restic.DataFile:
		n = Paths.Data
		if len(name) > 2 {
			n = filepath.Join(n, name[:2])
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
	return filepath.Join(base, n)
}

// Filename constructs the default path for given FileType and name.
func Filename(base string, t restic.FileType, name string) string {
	if t == restic.ConfigFile {
		return filepath.Join(base, "config")
	}

	return filepath.Join(Dirname(base, t, name), name)
}
