package rofs

import (
	"io"
	"io/fs"
	"slices"
	"time"

	"github.com/restic/restic/internal/debug"
)

// SnapshotsDir implements a tree of snapshots in repo as a file system in various sub-directories.
type SnapshotsDir struct {
	modTime time.Time

	pathTemplates []string
	timeTemplate  string

	// prepare the list of top-level directories
	entries []fs.DirEntry

	// used by ReadDir() with positive number of entries to return
	entriesRemaining []fs.DirEntry
}

// NewSnapshotsDir initializes a new top-level snapshots directory.
func NewSnapshotsDir(pathTemplates []string, timeTemplate string) *SnapshotsDir {
	dir := &SnapshotsDir{
		pathTemplates: pathTemplates,
		timeTemplate:  timeTemplate,
		modTime:       time.Now(),
	}

	testnames := []string{"foo", "bar", "baz", "snapshots"}
	for _, name := range testnames {
		dir.entries = append(dir.entries,
			fs.FileInfoToDirEntry(FileInfo{
				name:    name,
				mode:    0644,
				modtime: time.Now(),
			}))
	}

	slices.SortFunc(dir.entries, func(a, b fs.DirEntry) int {
		if a.Name() == b.Name() {
			return 0
		}

		if a.Name() < b.Name() {
			return 1
		}

		return -1
	})

	// prepare for readdir with positive n
	dir.entriesRemaining = dir.entries

	return dir
}

// ensure that it implements all necessary interfaces.
var _ fs.ReadDirFile = &SnapshotsDir{}

// Close closes the snapshots dir.
func (dir *SnapshotsDir) Close() error {
	debug.Log("Close()")

	// reset readdir list
	dir.entriesRemaining = dir.entries

	return nil
}

// Read is not implemented for a dir.
func (dir *SnapshotsDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{
		Op:  "read",
		Err: fs.ErrInvalid,
	}
}

// Stat returns information about the dir.
func (dir *SnapshotsDir) Stat() (fs.FileInfo, error) {
	debug.Log("Stat(root)")

	fi := FileInfo{
		name:    "root", // use special name, this is the root node
		size:    0,
		modtime: dir.modTime,
		mode:    0755,
	}

	return fi, nil
}

// ReadDir returns a list of entries.
func (dir *SnapshotsDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if n < 0 {
		debug.Log("Readdir(root, %v), return %v entries", n, len(dir.entries))
		return dir.entries, nil
	}

	// complicated pointer handling
	if n > len(dir.entriesRemaining) {
		n = len(dir.entriesRemaining)
	}

	if n == 0 {
		return nil, io.EOF
	}

	list := dir.entriesRemaining[:n]
	dir.entriesRemaining = dir.entriesRemaining[n:]

	return list, nil
}
