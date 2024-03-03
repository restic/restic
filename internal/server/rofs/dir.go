package rofs

import (
	"io"
	"io/fs"
	"slices"

	"github.com/restic/restic/internal/debug"
)

// dirEntry holds data for a file or directory.
type dirEntry struct {
	fileInfo fs.FileInfo
}

var _ fs.DirEntry = dirEntry{}

func (d dirEntry) Name() string               { return d.fileInfo.Name() }
func (d dirEntry) IsDir() bool                { return d.fileInfo.IsDir() }
func (d dirEntry) Type() fs.FileMode          { return d.fileInfo.Mode().Type() }
func (d dirEntry) Info() (fs.FileInfo, error) { return d.fileInfo, nil }

// openDir represents a directory opened for reading.
type openDir struct {
	path     string
	fileInfo fileInfo
	entries  []fs.DirEntry
	offset   int
}

var _ fs.ReadDirFile = &openDir{}

func (d *openDir) Close() error {
	debug.Log("Close(%v)", d.path)
	return nil
}

func (d *openDir) Stat() (fs.FileInfo, error) {
	debug.Log("Stat(%v)", d.path)
	return d.fileInfo, nil
}

func (d *openDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: fs.ErrInvalid}
}

func (d *openDir) ReadDir(count int) ([]fs.DirEntry, error) {
	n := len(d.entries) - d.offset

	if n == 0 && count > 0 {
		debug.Log("ReadDir(%v, %v) -> EOF", d.path, count)

		return nil, io.EOF
	}

	if count > 0 && n > count {
		n = count
	}

	list := make([]fs.DirEntry, 0, n)
	for i := 0; i < n; i++ {
		list = append(list, d.entries[d.offset+i])
	}

	d.offset += n

	debug.Log("ReadDir(%v, %v) -> %v entries", d.path, count, len(list))

	return list, nil
}

func dirMap2DirEntry(m map[string]rofsEntry) []fs.DirEntry {
	list := make([]fs.DirEntry, 0, len(m))

	for _, entry := range m {
		list = append(list, entry.DirEntry())
	}

	slices.SortFunc(list, func(a, b fs.DirEntry) int {
		if a.Name() == b.Name() {
			return 0
		}

		if a.Name() < b.Name() {
			return -1
		}

		return 1
	})

	return list
}
