package fs

import (
	"os"
	"syscall"

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

func openReadHandle(path string, flag int) (*os.File, error) {
	f, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return nil, err
	}
	_ = setFlags(f)
	return f, nil
}

// reopenMetadataHandle reopens a handle created by openMetadataHandle for reading.
// The caller must no longer use the original file.
func reopenMetadataHandle(f *os.File) (*os.File, error) {
	defer func() {
		_ = f.Close()
	}()

	f2, err := os.OpenFile(linuxFdPath(f.Fd()), O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f2.Close()
	}()

	// Duplicate the filehandle and use that to create a file object with the correct name.
	// f2 will automatically close its file handle on garbage collection.
	fd3, err := syscall.Dup(int(f2.Fd()))
	if err != nil {
		return nil, err
	}

	f3 := os.NewFile(uintptr(fd3), f.Name())
	_ = setFlags(f3)
	return f3, nil
}
