package fs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/windows"
)

// GetSecurityDescriptor takes the path of the file and returns the SecurityDescriptor for the file.
// This needs admin permissions or SeBackupPrivilege for getting the full SD.
// If there are no admin permissions, only the current user's owner, group and DACL will be got.
func GetSecurityDescriptor(filePath string) (securityDescriptor *[]byte, err error) {
	onceBackup.Do(enableBackupPrivilege)

	var sd *windows.SECURITY_DESCRIPTOR

	if lowerPrivileges {
		sd, err = getNamedSecurityInfoLow(sd, err, filePath)
	} else {
		sd, err = getNamedSecurityInfoHigh(sd, err, filePath)
	}
	if err != nil {
		if isHandlePrivilegeNotHeldError(err) {
			lowerPrivileges = true
			sd, err = getNamedSecurityInfoLow(sd, err, filePath)
			if err != nil {
				return nil, fmt.Errorf("get low-level named security info failed with: %w", err)
			}
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

// SetSecurityDescriptor sets the SecurityDescriptor for the file at the specified path.
// This needs admin permissions or SeRestorePrivilege, SeSecurityPrivilege and SeTakeOwnershipPrivilege
// for setting the full SD.
// If there are no admin permissions/required privileges, only the DACL from the SD can be set and
// owner and group will be set based on the current user.
func SetSecurityDescriptor(filePath string, securityDescriptor *[]byte) error {
	onceRestore.Do(enableRestorePrivilege)
	// Set the security descriptor on the file
	sd, err := SecurityDescriptorBytesToStruct(*securityDescriptor)
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

	if lowerPrivileges {
		err = setNamedSecurityInfoLow(filePath, dacl)
	} else {
		err = setNamedSecurityInfoHigh(filePath, owner, group, dacl, sacl)
	}

	if err != nil {
		if isHandlePrivilegeNotHeldError(err) {
			lowerPrivileges = true
			err = setNamedSecurityInfoLow(filePath, dacl)
			if err != nil {
				return fmt.Errorf("set low-level named security info failed with: %w", err)
			}
		} else {
			return fmt.Errorf("set named security info failed with: %w", err)
		}
	}
	return nil
}

var (
	onceBackup  sync.Once
	onceRestore sync.Once

	// SeBackupPrivilege allows the application to bypass file and directory ACLs to back up files and directories.
	SeBackupPrivilege = "SeBackupPrivilege"
	// SeRestorePrivilege allows the application to bypass file and directory ACLs to restore files and directories.
	SeRestorePrivilege = "SeRestorePrivilege"
	// SeSecurityPrivilege allows read and write access to all SACLs.
	SeSecurityPrivilege = "SeSecurityPrivilege"
	// SeTakeOwnershipPrivilege allows the application to take ownership of files and directories, regardless of the permissions set on them.
	SeTakeOwnershipPrivilege = "SeTakeOwnershipPrivilege"

	backupPrivilegeError  error
	restorePrivilegeError error
	lowerPrivileges       bool
)

// Flags for backup and restore with admin permissions
var highSecurityFlags windows.SECURITY_INFORMATION = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.SACL_SECURITY_INFORMATION | windows.LABEL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.SCOPE_SECURITY_INFORMATION | windows.BACKUP_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION | windows.PROTECTED_SACL_SECURITY_INFORMATION

// Flags for backup without admin permissions. If there are no admin permissions, only the current user's owner, group and DACL will be backed up.
var lowBackupSecurityFlags windows.SECURITY_INFORMATION = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.LABEL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.SCOPE_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION

// Flags for restore without admin permissions. If there are no admin permissions, only the DACL from the SD can be restored and owner and group will be set based on the current user.
var lowRestoreSecurityFlags windows.SECURITY_INFORMATION = windows.DACL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION

// getNamedSecurityInfoHigh gets the higher level SecurityDescriptor which requires admin permissions.
func getNamedSecurityInfoHigh(sd *windows.SECURITY_DESCRIPTOR, err error, filePath string) (*windows.SECURITY_DESCRIPTOR, error) {
	return windows.GetNamedSecurityInfo(filePath, windows.SE_FILE_OBJECT, highSecurityFlags)
}

// getNamedSecurityInfoLow gets the lower level SecurityDescriptor which requires no admin permissions.
func getNamedSecurityInfoLow(sd *windows.SECURITY_DESCRIPTOR, err error, filePath string) (*windows.SECURITY_DESCRIPTOR, error) {
	return windows.GetNamedSecurityInfo(filePath, windows.SE_FILE_OBJECT, lowBackupSecurityFlags)
}

// setNamedSecurityInfoHigh sets the higher level SecurityDescriptor which requires admin permissions.
func setNamedSecurityInfoHigh(filePath string, owner *windows.SID, group *windows.SID, dacl *windows.ACL, sacl *windows.ACL) error {
	return windows.SetNamedSecurityInfo(filePath, windows.SE_FILE_OBJECT, highSecurityFlags, owner, group, dacl, sacl)
}

// setNamedSecurityInfoLow sets the lower level SecurityDescriptor which requires no admin permissions.
func setNamedSecurityInfoLow(filePath string, dacl *windows.ACL) error {
	return windows.SetNamedSecurityInfo(filePath, windows.SE_FILE_OBJECT, lowRestoreSecurityFlags, nil, nil, dacl, nil)
}

// enableBackupPrivilege enables privilege for backing up security descriptors
func enableBackupPrivilege() {
	err := enableProcessPrivileges([]string{SeBackupPrivilege})
	if err != nil {
		backupPrivilegeError = fmt.Errorf("error enabling backup privilege: %w", err)
	}
}

// enableBackupPrivilege enables privilege for restoring security descriptors
func enableRestorePrivilege() {
	err := enableProcessPrivileges([]string{SeRestorePrivilege, SeSecurityPrivilege, SeTakeOwnershipPrivilege})
	if err != nil {
		restorePrivilegeError = fmt.Errorf("error enabling restore/security privilege: %w", err)
	}
}

// DisableBackupPrivileges disables privileges that are needed for backup operations.
// They may be reenabled if GetSecurityDescriptor is called again.
func DisableBackupPrivileges() error {
	//Reset the once so that backup privileges can be enabled again if needed.
	onceBackup = sync.Once{}
	return enableDisableProcessPrivilege([]string{SeBackupPrivilege}, 0)
}

// DisableRestorePrivileges disables privileges that are needed for restore operations.
// They may be reenabled if SetSecurityDescriptor is called again.
func DisableRestorePrivileges() error {
	//Reset the once so that restore privileges can be enabled again if needed.
	onceRestore = sync.Once{}
	return enableDisableProcessPrivilege([]string{SeRestorePrivilege, SeSecurityPrivilege}, 0)
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

// IsAdmin checks if current user is an administrator.
func IsAdmin() (isAdmin bool, err error) {
	var sid *windows.SID
	err = windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY, 2, windows.SECURITY_BUILTIN_DOMAIN_RID, windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return false, errors.Errorf("sid error: %s", err)
	}
	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false, errors.Errorf("token membership error: %s", err)
	}
	return member, nil
}

// The code below was adapted from github.com/Microsoft/go-winio under MIT license.

// The MIT License (MIT)

// Copyright (c) 2015 Microsoft

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
var (
	modadvapi32 = windows.NewLazySystemDLL("advapi32.dll")

	procLookupPrivilegeValueW       = modadvapi32.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivileges       = modadvapi32.NewProc("AdjustTokenPrivileges")
	procLookupPrivilegeDisplayNameW = modadvapi32.NewProc("LookupPrivilegeDisplayNameW")
	procLookupPrivilegeNameW        = modadvapi32.NewProc("LookupPrivilegeNameW")
)

// Do the interface allocations only once for common
// Errno values.
const (
	errnoErrorIOPending = 997

	//revive:disable-next-line:var-naming ALL_CAPS
	SE_PRIVILEGE_ENABLED = windows.SE_PRIVILEGE_ENABLED

	//revive:disable-next-line:var-naming ALL_CAPS
	ERROR_NOT_ALL_ASSIGNED syscall.Errno = windows.ERROR_NOT_ALL_ASSIGNED
)

var (
	errErrorIOPending error = syscall.Errno(errnoErrorIOPending)
	errErrorEinval    error = syscall.EINVAL

	privNames     = make(map[string]uint64)
	privNameMutex sync.Mutex
)

// PrivilegeError represents an error enabling privileges.
type PrivilegeError struct {
	privileges []uint64
}

// SecurityDescriptorBytesToStruct converts the security descriptor bytes representation
// into a pointer to windows SECURITY_DESCRIPTOR.
func SecurityDescriptorBytesToStruct(sd []byte) (*windows.SECURITY_DESCRIPTOR, error) {
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

// Error returns the string message for the error.
func (e *PrivilegeError) Error() string {
	s := "Could not enable privilege "
	if len(e.privileges) > 1 {
		s = "Could not enable privileges "
	}
	for i, p := range e.privileges {
		if i != 0 {
			s += ", "
		}
		s += `"`
		s += getPrivilegeName(p)
		s += `"`
	}
	if backupPrivilegeError != nil {
		s += " backupPrivilegeError:" + backupPrivilegeError.Error()
	}
	if restorePrivilegeError != nil {
		s += " restorePrivilegeError:" + restorePrivilegeError.Error()
	}
	return s
}

func mapPrivileges(names []string) ([]uint64, error) {
	privileges := make([]uint64, 0, len(names))
	privNameMutex.Lock()
	defer privNameMutex.Unlock()
	for _, name := range names {
		p, ok := privNames[name]
		if !ok {
			err := lookupPrivilegeValue("", name, &p)
			if err != nil {
				return nil, err
			}
			privNames[name] = p
		}
		privileges = append(privileges, p)
	}
	return privileges, nil
}

// enableProcessPrivileges enables privileges globally for the process.
func enableProcessPrivileges(names []string) error {
	return enableDisableProcessPrivilege(names, SE_PRIVILEGE_ENABLED)
}

// DisableProcessPrivileges disables privileges globally for the process.
func DisableProcessPrivileges(names []string) error {
	return enableDisableProcessPrivilege(names, 0)
}

func enableDisableProcessPrivilege(names []string, action uint32) error {
	privileges, err := mapPrivileges(names)
	if err != nil {
		return err
	}

	p := windows.CurrentProcess()
	var token windows.Token
	err = windows.OpenProcessToken(p, windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return err
	}

	defer func() {
		_ = token.Close()
	}()
	return adjustPrivileges(token, privileges, action)
}

func adjustPrivileges(token windows.Token, privileges []uint64, action uint32) error {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(privileges)))
	for _, p := range privileges {
		_ = binary.Write(&b, binary.LittleEndian, p)
		_ = binary.Write(&b, binary.LittleEndian, action)
	}
	prevState := make([]byte, b.Len())
	reqSize := uint32(0)
	success, err := adjustTokenPrivileges(token, false, &b.Bytes()[0], uint32(len(prevState)), &prevState[0], &reqSize)
	if !success {
		return err
	}
	if err == ERROR_NOT_ALL_ASSIGNED { //nolint:errorlint // err is Errno
		return &PrivilegeError{privileges}
	}
	return nil
}

func getPrivilegeName(luid uint64) string {
	var nameBuffer [256]uint16
	bufSize := uint32(len(nameBuffer))
	err := lookupPrivilegeName("", &luid, &nameBuffer[0], &bufSize)
	if err != nil {
		return fmt.Sprintf("<unknown privilege %d>", luid)
	}

	var displayNameBuffer [256]uint16
	displayBufSize := uint32(len(displayNameBuffer))
	var langID uint32
	err = lookupPrivilegeDisplayName("", &nameBuffer[0], &displayNameBuffer[0], &displayBufSize, &langID)
	if err != nil {
		return fmt.Sprintf("<unknown privilege %s>", string(utf16.Decode(nameBuffer[:bufSize])))
	}

	return string(utf16.Decode(displayNameBuffer[:displayBufSize]))
}

func adjustTokenPrivileges(token windows.Token, releaseAll bool, input *byte, outputSize uint32, output *byte, requiredSize *uint32) (success bool, err error) {
	var _p0 uint32
	if releaseAll {
		_p0 = 1
	}
	r0, _, e1 := syscall.SyscallN(procAdjustTokenPrivileges.Addr(), uintptr(token), uintptr(_p0), uintptr(unsafe.Pointer(input)), uintptr(outputSize), uintptr(unsafe.Pointer(output)), uintptr(unsafe.Pointer(requiredSize)))
	success = r0 != 0
	if !success {
		err = errnoErr(e1)
	}
	return
}

func lookupPrivilegeDisplayName(systemName string, name *uint16, buffer *uint16, size *uint32, languageID *uint32) (err error) {
	var _p0 *uint16
	_p0, err = syscall.UTF16PtrFromString(systemName)
	if err != nil {
		return
	}
	return _lookupPrivilegeDisplayName(_p0, name, buffer, size, languageID)
}

func _lookupPrivilegeDisplayName(systemName *uint16, name *uint16, buffer *uint16, size *uint32, languageID *uint32) (err error) {
	r1, _, e1 := syscall.SyscallN(procLookupPrivilegeDisplayNameW.Addr(), uintptr(unsafe.Pointer(systemName)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(buffer)), uintptr(unsafe.Pointer(size)), uintptr(unsafe.Pointer(languageID)), 0)
	if r1 == 0 {
		err = errnoErr(e1)
	}
	return
}

func lookupPrivilegeName(systemName string, luid *uint64, buffer *uint16, size *uint32) (err error) {
	var _p0 *uint16
	_p0, err = syscall.UTF16PtrFromString(systemName)
	if err != nil {
		return
	}
	return _lookupPrivilegeName(_p0, luid, buffer, size)
}

func _lookupPrivilegeName(systemName *uint16, luid *uint64, buffer *uint16, size *uint32) (err error) {
	r1, _, e1 := syscall.SyscallN(procLookupPrivilegeNameW.Addr(), uintptr(unsafe.Pointer(systemName)), uintptr(unsafe.Pointer(luid)), uintptr(unsafe.Pointer(buffer)), uintptr(unsafe.Pointer(size)), 0, 0)
	if r1 == 0 {
		err = errnoErr(e1)
	}
	return
}

func lookupPrivilegeValue(systemName string, name string, luid *uint64) (err error) {
	var _p0 *uint16
	_p0, err = syscall.UTF16PtrFromString(systemName)
	if err != nil {
		return
	}
	var _p1 *uint16
	_p1, err = syscall.UTF16PtrFromString(name)
	if err != nil {
		return
	}
	return _lookupPrivilegeValue(_p0, _p1, luid)
}

func _lookupPrivilegeValue(systemName *uint16, name *uint16, luid *uint64) (err error) {
	r1, _, e1 := syscall.SyscallN(procLookupPrivilegeValueW.Addr(), uintptr(unsafe.Pointer(systemName)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(luid)))
	if r1 == 0 {
		err = errnoErr(e1)
	}
	return
}

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return errErrorEinval
	case errnoErrorIOPending:
		return errErrorIOPending
	}
	// TODO: add more here, after collecting data on the common
	// error values see on Windows. (perhaps when running
	// all.bat?)
	return e
}
