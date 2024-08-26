//go:build aix
// +build aix

package fs

import (
	"os"
	"syscall"

	"github.com/restic/restic/internal/restic"
)

func nodeRestoreSymlinkTimestamps(_ string, _ [2]syscall.Timespec) error {
	return nil
}

// AIX has a funny timespec type in syscall, with 32-bit nanoseconds.
// golang.org/x/sys/unix handles this cleanly, but we're stuck with syscall
// because os.Stat returns a syscall type in its os.FileInfo.Sys().
func toTimespec(t syscall.StTimespec_t) syscall.Timespec {
	return syscall.Timespec{Sec: t.Sec, Nsec: int64(t.Nsec)}
}

func (s statT) atim() syscall.Timespec { return toTimespec(s.Atim) }
func (s statT) mtim() syscall.Timespec { return toTimespec(s.Mtim) }
func (s statT) ctim() syscall.Timespec { return toTimespec(s.Ctim) }

// nodeRestoreExtendedAttributes is a no-op on AIX.
func nodeRestoreExtendedAttributes(_ *restic.Node, _ string) error {
	return nil
}

// nodeFillExtendedAttributes is a no-op on AIX.
func nodeFillExtendedAttributes(_ *restic.Node, _ string, _ bool) error {
	return nil
}

// IsListxattrPermissionError is a no-op on AIX.
func IsListxattrPermissionError(_ error) bool {
	return false
}

// nodeRestoreGenericAttributes is no-op on AIX.
func nodeRestoreGenericAttributes(node *restic.Node, _ string, warn func(msg string)) error {
	return restic.HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// nodeFillGenericAttributes is a no-op on AIX.
func nodeFillGenericAttributes(_ *restic.Node, _ string, _ os.FileInfo, _ *statT) (allowExtended bool, err error) {
	return true, nil
}
