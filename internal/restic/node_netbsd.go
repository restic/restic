package restic

import (
	"os"
	"syscall"
)

func (node Node) restoreSymlinkTimestamps(_ string, _ [2]syscall.Timespec) error {
	return nil
}

func (s statT) atim() syscall.Timespec { return s.Atimespec }
func (s statT) mtim() syscall.Timespec { return s.Mtimespec }
func (s statT) ctim() syscall.Timespec { return s.Ctimespec }

// Getxattr is a no-op on netbsd.
func Getxattr(path, name string) ([]byte, error) {
	return nil, nil
}

// Listxattr is a no-op on netbsd.
func Listxattr(path string) ([]string, error) {
	return nil, nil
}

// Setxattr is a no-op on netbsd.
func Setxattr(path, name string, data []byte) error {
	return nil
}

// restoreGenericAttributes is no-op on netbsd.
func (node *Node) restoreGenericAttributes(_ string, warn func(msg string)) error {
	return node.handleAllUnknownGenericAttributesFound(warn)
}

// fillGenericAttributes is a no-op on netbsd.
func (node *Node) fillGenericAttributes(_ string, _ os.FileInfo, _ *statT) (allowExtended bool, err error) {
	return true, nil
}
