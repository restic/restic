package fs

import (
	"io"
	"os"

	"github.com/pkg/sftp"
	"github.com/restic/restic/internal/errors"
)

// SftpFile extends the sftp.File to implements File
type SftpFile struct {
	*sftp.File

	client    *sftp.Client
	fis       []os.FileInfo
	isReaddir bool
}

// statically ensure SftpFile implements File
var _ File = &SftpFile{}

var (
	// ErrNotImplemented means this method is not implemented
	ErrNotImplemented = errors.New("this method is not implemented")
)

// Fd returns the integer Unix file descriptor referencing the open file.
// The file descriptor is valid only until f.Close is called or f is garbage
// collected. On Unix systems this will cause the SetDeadline methods to
// stop working.
// Here only return 0
func (f *SftpFile) Fd() uintptr {
	return ^(uintptr(0))
}

// Readdir reads the contents of the directory associated with file and returns
// a slice of up to n FileInfo values, as would be returned by Lstat,
// in directory order. Subsequent calls on the same file will yield further FileInfos.
//
// If n > 0, Readdir returns at most n FileInfo structures. In this case, if Readdir
// returns an empty slice, it will return a non-nil error explaining why. At the end
// of a directory, the error is io.EOF.
//
// If n <= 0, Readdir returns all the FileInfo from the directory in a single slice.
// In this case, if Readdir succeeds (reads all the way to the end of the directory),
// it returns the slice and a nil error. If it encounters an error before the end of the directory, Readdir returns the FileInfo read until that point and a non-nil error.
func (f *SftpFile) Readdir(n int) ([]os.FileInfo, error) {
	if !f.isReaddir {
		fis, err := f.client.ReadDir(f.Name())
		if err != nil {
			return []os.FileInfo{}, err
		}

		f.fis = fis
		f.isReaddir = true
	}

	if n <= 0 {
		// get a new slice
		fis := f.fis[:]
		// set the length of this slice to 0
		f.fis = f.fis[len(f.fis):]
		return fis, nil
	}

	if n > len(f.fis) {
		f.fis = f.fis[len(f.fis):]
		return f.fis[:], io.EOF
	}

	fis := f.fis[:n]
	f.fis = f.fis[n:]
	return fis, nil
}

// Readdirnames reads the contents of the directory associated with file and
// returns a slice of up to n names of files in the directory, in directory order.
// Subsequent calls on the same file will yield further names.
//
// If n > 0, Readdirnames returns at most n names. In this case,
// if Readdirnames returns an empty slice, it will return a non-nil error explaining why.
// At the end of a directory, the error is io.EOF.
//
// If n <= 0, Readdirnames returns all the names from the directory in a single slice.
// In this case, if Readdirnames succeeds (reads all the way to the end of the directory), it returns the slice and a nil error. If it encounters an error before the end of the directory, Readdirnames returns the names read until that point and a non-nil error.
func (f *SftpFile) Readdirnames(n int) ([]string, error) {
	fis, err := f.Readdir(n)
	if err != nil {
		return []string{}, err
	}

	names := make([]string, 0, len(fis))
	for _, fi := range fis {
		names = append(names, fi.Name())
	}

	return names, nil
}
