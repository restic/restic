package fs

import (
	"io"
	"os"
)

// File is an open file on a file system.
type File interface {
	io.Reader
	io.Writer
	io.Closer

	Stat() (os.FileInfo, error)

	// ClearCache removes the file's content from the OS cache.
	ClearCache() error
}
