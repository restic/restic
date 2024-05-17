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

// restoreExtendedAttributes is a no-op on netbsd.
func (node Node) restoreExtendedAttributes(_ string) error {
	return nil
}

// fillExtendedAttributes is a no-op on netbsd.
func (node *Node) fillExtendedAttributes(_ string, _ bool) error {
	return nil
}

// IsListxattrPermissionError is a no-op on netbsd.
func IsListxattrPermissionError(_ error) bool {
	return false
}

// restoreGenericAttributes is no-op on netbsd.
func (node *Node) restoreGenericAttributes(_ string, warn func(msg string)) error {
	return node.handleAllUnknownGenericAttributesFound(warn)
}

// fillGenericAttributes is a no-op on netbsd.
func (node *Node) fillGenericAttributes(_ string, _ os.FileInfo, _ *statT) (allowExtended bool, err error) {
	return true, nil
}
