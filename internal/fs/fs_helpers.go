package fs

import "os"

// ReadDir reads the directory named by dirname within fs and returns a list of
// directory entries.
func ReadDir(fs FS, dirname string) ([]os.FileInfo, error) {
	f, err := fs.Open(dirname)
	if err != nil {
		return nil, err
	}

	entries, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// ReadDirNames reads the directory named by dirname within fs and returns a
// list of entry names.
func ReadDirNames(fs FS, dirname string) ([]string, error) {
	f, err := fs.Open(dirname)
	if err != nil {
		return nil, err
	}

	entries, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return entries, nil
}
