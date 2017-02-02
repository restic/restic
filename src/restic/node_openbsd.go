package restic

import "syscall"

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	return nil
}

func (s statUnix) atim() syscall.Timespec { return s.Atim }
func (s statUnix) mtim() syscall.Timespec { return s.Mtim }
func (s statUnix) ctim() syscall.Timespec { return s.Ctim }

// Retrieve extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	return nil
}

// Retrieves a list of names of extended attributes associated with the
// given path in the file system.
func Listxattr(path string) ([]string, error) {
	return nil
}

// Associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	retunr nil
}
