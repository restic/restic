package fs

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
)

// Track is a wrapper around another file system which installs finalizers
// for open files which call panic() when they are not closed when the garbage
// collector releases them. This can be used to find resource leaks via open
// files.
type Track struct {
	FS
}

// OpenFile wraps the OpenFile method of the underlying file system.
func (fs Track) OpenFile(name string, flag int, metadataOnly bool) (File, error) {
	f, err := fs.FS.OpenFile(name, flag, metadataOnly)
	if err != nil {
		return nil, err
	}

	return newTrackFile(debug.Stack(), name, f), nil
}

type trackFile struct {
	File
}

func newTrackFile(stack []byte, filename string, file File) *trackFile {
	f := &trackFile{file}
	runtime.SetFinalizer(f, func(_ any) {
		fmt.Fprintf(os.Stderr, "file %s not closed\n\nStacktrack:\n%s\n", filename, stack)
		panic("file " + filename + " not closed")
	})
	return f
}

func (f *trackFile) Close() error {
	runtime.SetFinalizer(f, nil)
	return f.File.Close()
}
