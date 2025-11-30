//go:build windows

package fs

import (
	"github.com/Microsoft/go-winio"
	"github.com/restic/restic/internal/errors"
)

var processPrivileges = []string{
	// seBackupPrivilege allows the application to bypass file and directory ACLs to back up files and directories.
	"SeBackupPrivilege",
	// seRestorePrivilege allows the application to bypass file and directory ACLs to restore files and directories.
	"SeRestorePrivilege",
	// seSecurityPrivilege allows read and write access to all SACLs.
	"SeSecurityPrivilege",
	// seTakeOwnershipPrivilege allows the application to take ownership of files and directories, regardless of the permissions set on them.
	"SeTakeOwnershipPrivilege",
}

// enableProcessPrivileges enables additional file system privileges for the current process.
func enableProcessPrivileges() error {
	var errs []error

	// EnableProcessPrivileges may enable some but not all requested privileges, yet its error lists all requested.
	// Request one at a time to return what actually fails.
	for _, p := range processPrivileges {
		errs = append(errs, winio.EnableProcessPrivileges([]string{p}))
	}

	return errors.Join(errs...)
}
