//go:build aix
// +build aix

package restic

import "syscall"

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
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

// Getxattr is a no-op on AIX.
func Getxattr(path, name string) ([]byte, error) {
	return nil, nil
}

// Listxattr is a no-op on AIX.
func Listxattr(path string) ([]string, error) {
	return nil, nil
}

// Setxattr is a no-op on AIX.
func Setxattr(path, name string, data []byte) error {
	return nil
}
