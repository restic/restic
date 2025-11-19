//go:build !windows
// +build !windows

package fs

// enableProcessPrivileges enables additional file system privileges for the current process.
func enableProcessPrivileges() error {
	return nil
}
