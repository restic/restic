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

	Readdirnames(n int) ([]string, error)
	Readdir(int) ([]os.FileInfo, error)
	Seek(int64, int) (int64, error)
	Stat() (os.FileInfo, error)
}
