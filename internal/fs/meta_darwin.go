package fs

import (
	"os"

	"golang.org/x/sys/unix"
)

func openMetadataHandle(path string, flag int) (*os.File, error) {
	flags := O_RDONLY
	if flag&O_NOFOLLOW != 0 {
		// open symlink instead of following it
		flags |= unix.O_SYMLINK
	}
	if flag&O_DIRECTORY != 0 {
		flags |= O_DIRECTORY
	}

	f, err := os.OpenFile(path, flags, 0)
	if err != nil {
		return nil, err
	}
	_ = setFlags(f)
	return f, nil
}
