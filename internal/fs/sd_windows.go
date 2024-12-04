package fs

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/windows"
)

var (
	onceBackup  sync.Once
	onceRestore sync.Once

	// seBackupPrivilege allows the application to bypass file and directory ACLs to back up files and directories.
	seBackupPrivilege = "SeBackupPrivilege"
	// seRestorePrivilege allows the application to bypass file and directory ACLs to restore files and directories.
	seRestorePrivilege = "SeRestorePrivilege"
	// seSecurityPrivilege allows read and write access to all SACLs.
	seSecurityPrivilege = "SeSecurityPrivilege"
	// seTakeOwnershipPrivilege allows the application to take ownership of files and directories, regardless of the permissions set on them.
	seTakeOwnershipPrivilege = "SeTakeOwnershipPrivilege"

	lowerPrivileges atomic.Bool
)

// Flags for backup and restore with admin permissions
var highSecurityFlags windows.SECURITY_INFORMATION = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.SACL_SECURITY_INFORMATION | windows.LABEL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.SCOPE_SECURITY_INFORMATION | windows.BACKUP_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION | windows.PROTECTED_SACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION | windows.UNPROTECTED_SACL_SECURITY_INFORMATION

// Flags for backup without admin permissions. If there are no admin permissions, only the current user's owner, group and DACL will be backed up.
var lowBackupSecurityFlags windows.SECURITY_INFORMATION = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.LABEL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.SCOPE_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION

// Flags for restore without admin permissions. If there are no admin permissions, only the DACL from the SD can be restored and owner and group will be set based on the current user.
var lowRestoreSecurityFlags windows.SECURITY_INFORMATION = windows.DACL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION

// getSecurityDescriptor takes the path of the file and returns the SecurityDescriptor for the file.
// This needs admin permissions or SeBackupPrivilege for getting the full SD.
// If there are no admin permissions, only the current user's owner, group and DACL will be got.
func getSecurityDescriptor(filePath string) (securityDescriptor *[]byte, err error) {
	onceBackup.Do(enableBackupPrivilege)

	var sd *windows.SECURITY_DESCRIPTOR

	// store original value to avoid unrelated changes in the error check
	useLowerPrivileges := lowerPrivileges.Load()
	if useLowerPrivileges {
		sd, err = getNamedSecurityInfoLow(filePath)
	} else {
		sd, err = getNamedSecurityInfoHigh(filePath)
		// Fallback to the low privilege version when receiving an access denied error.
		// For some reason the ERROR_PRIVILEGE_NOT_HELD error is not returned for removable media
		// but instead an access denied error is returned. Workaround that by just retrying with
		// the low privilege version, but don't switch privileges as we cannot distinguish this
		// case from actual access denied errors.
		// see https://github.com/restic/restic/issues/5003#issuecomment-2452314191 for details
		if err != nil && isAccessDeniedError(err) {
			sd, err = getNamedSecurityInfoLow(filePath)
		}
	}
	if err != nil {
		if !useLowerPrivileges && isHandlePrivilegeNotHeldError(err) {
			// If ERROR_PRIVILEGE_NOT_HELD is encountered, fallback to backups/restores using lower non-admin privileges.
			lowerPrivileges.Store(true)
			return getSecurityDescriptor(filePath)
		} else if errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
			return nil, nil
		} else {
			return nil, fmt.Errorf("get named security info failed with: %w", err)
		}
	}

	sdBytes, err := securityDescriptorStructToBytes(sd)
	if err != nil {
		return nil, fmt.Errorf("convert security descriptor to bytes failed: %w", err)
	}
	return &sdBytes, nil
}

// setSecurityDescriptor sets the SecurityDescriptor for the file at the specified path.
// This needs admin permissions or SeRestorePrivilege, SeSecurityPrivilege and SeTakeOwnershipPrivilege
// for setting the full SD.
// If there are no admin permissions/required privileges, only the DACL from the SD can be set and
// owner and group will be set based on the current user.
func setSecurityDescriptor(filePath string, securityDescriptor *[]byte) error {
	onceRestore.Do(enableRestorePrivilege)
	// Set the security descriptor on the file
	sd, err := securityDescriptorBytesToStruct(*securityDescriptor)
	if err != nil {
		return fmt.Errorf("error converting bytes to security descriptor: %w", err)
	}

	owner, _, err := sd.Owner()
	if err != nil {
		//Do not set partial values.
		owner = nil
	}
	group, _, err := sd.Group()
	if err != nil {
		//Do not set partial values.
		group = nil
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		//Do not set partial values.
		dacl = nil
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		//Do not set partial values.
		sacl = nil
	}

	// store original value to avoid unrelated changes in the error check
	useLowerPrivileges := lowerPrivileges.Load()
	if useLowerPrivileges {
		err = setNamedSecurityInfoLow(filePath, dacl)
	} else {
		err = setNamedSecurityInfoHigh(filePath, owner, group, dacl, sacl)
		// See corresponding fallback in getSecurityDescriptor for an explanation
		if err != nil && isAccessDeniedError(err) {
			err = setNamedSecurityInfoLow(filePath, dacl)
		}
	}

	if err != nil {
		if !useLowerPrivileges && isHandlePrivilegeNotHeldError(err) {
			// If ERROR_PRIVILEGE_NOT_HELD is encountered, fallback to backups/restores using lower non-admin privileges.
			lowerPrivileges.Store(true)
			return setSecurityDescriptor(filePath, securityDescriptor)
		} else {
			return fmt.Errorf("set named security info failed with: %w", err)
		}
	}
	return nil
}

// getNamedSecurityInfoHigh gets the higher level SecurityDescriptor which requires admin permissions.
func getNamedSecurityInfoHigh(filePath string) (*windows.SECURITY_DESCRIPTOR, error) {
	return windows.GetNamedSecurityInfo(fixpath(filePath), windows.SE_FILE_OBJECT, highSecurityFlags)
}

// getNamedSecurityInfoLow gets the lower level SecurityDescriptor which requires no admin permissions.
func getNamedSecurityInfoLow(filePath string) (*windows.SECURITY_DESCRIPTOR, error) {
	return windows.GetNamedSecurityInfo(fixpath(filePath), windows.SE_FILE_OBJECT, lowBackupSecurityFlags)
}

// setNamedSecurityInfoHigh sets the higher level SecurityDescriptor which requires admin permissions.
func setNamedSecurityInfoHigh(filePath string, owner *windows.SID, group *windows.SID, dacl *windows.ACL, sacl *windows.ACL) error {
	return windows.SetNamedSecurityInfo(fixpath(filePath), windows.SE_FILE_OBJECT, highSecurityFlags, owner, group, dacl, sacl)
}

// setNamedSecurityInfoLow sets the lower level SecurityDescriptor which requires no admin permissions.
func setNamedSecurityInfoLow(filePath string, dacl *windows.ACL) error {
	return windows.SetNamedSecurityInfo(fixpath(filePath), windows.SE_FILE_OBJECT, lowRestoreSecurityFlags, nil, nil, dacl, nil)
}

func enableProcessPrivileges(privileges []string) error {
	return winio.EnableProcessPrivileges(privileges)
}

// enableBackupPrivilege enables privilege for backing up security descriptors
func enableBackupPrivilege() {
	err := enableProcessPrivileges([]string{seBackupPrivilege})
	if err != nil {
		debug.Log("error enabling backup privilege: %v", err)
	}
}

// enableRestorePrivilege enables privilege for restoring security descriptors
func enableRestorePrivilege() {
	err := enableProcessPrivileges([]string{seRestorePrivilege, seSecurityPrivilege, seTakeOwnershipPrivilege})
	if err != nil {
		debug.Log("error enabling restore/security privilege: %v", err)
	}
}

// isHandlePrivilegeNotHeldError checks if the error is ERROR_PRIVILEGE_NOT_HELD
func isHandlePrivilegeNotHeldError(err error) bool {
	// Use a type assertion to check if the error is of type syscall.Errno
	if errno, ok := err.(syscall.Errno); ok {
		// Compare the error code to the expected value
		return errno == windows.ERROR_PRIVILEGE_NOT_HELD
	}
	return false
}

// isAccessDeniedError checks if the error is ERROR_ACCESS_DENIED
func isAccessDeniedError(err error) bool {
	if errno, ok := err.(syscall.Errno); ok {
		// Compare the error code to the expected value
		return errno == windows.ERROR_ACCESS_DENIED
	}
	return false
}

// securityDescriptorBytesToStruct converts the security descriptor bytes representation
// into a pointer to windows SECURITY_DESCRIPTOR.
func securityDescriptorBytesToStruct(sd []byte) (*windows.SECURITY_DESCRIPTOR, error) {
	if l := int(unsafe.Sizeof(windows.SECURITY_DESCRIPTOR{})); len(sd) < l {
		return nil, fmt.Errorf("securityDescriptor (%d) smaller than expected (%d): %w", len(sd), l, windows.ERROR_INCORRECT_SIZE)
	}
	s := (*windows.SECURITY_DESCRIPTOR)(unsafe.Pointer(&sd[0]))
	return s, nil
}

// securityDescriptorStructToBytes converts the pointer to windows SECURITY_DESCRIPTOR
// into a security descriptor bytes representation.
func securityDescriptorStructToBytes(sd *windows.SECURITY_DESCRIPTOR) ([]byte, error) {
	b := unsafe.Slice((*byte)(unsafe.Pointer(sd)), sd.Length())
	return b, nil
}
