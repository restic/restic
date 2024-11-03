package fs

import (
	"os"

	"golang.org/x/sys/unix"
)

func openMetadataHandle(path string, flag int) (*os.File, error) {
	// O_PATH|O_NOFOLLOW is necessary to also be able to get a handle to symlinks
	flags := unix.O_PATH
	if flag&O_NOFOLLOW != 0 {
		flags |= O_NOFOLLOW
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
