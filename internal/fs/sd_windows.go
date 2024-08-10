package fs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/windows"
)

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

	lowerPrivileges atomic.Bool
)

// Flags for backup and restore with admin permissions
var highSecurityFlags windows.SECURITY_INFORMATION = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.SACL_SECURITY_INFORMATION | windows.LABEL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.SCOPE_SECURITY_INFORMATION | windows.BACKUP_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION | windows.PROTECTED_SACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION | windows.UNPROTECTED_SACL_SECURITY_INFORMATION

// Flags for backup without admin permissions. If there are no admin permissions, only the current user's owner, group and DACL will be backed up.
var lowBackupSecurityFlags windows.SECURITY_INFORMATION = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.LABEL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.SCOPE_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION

// Flags for restore without admin permissions. If there are no admin permissions, only the DACL from the SD can be restored and owner and group will be set based on the current user.
var lowRestoreSecurityFlags windows.SECURITY_INFORMATION = windows.DACL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION

// GetSecurityDescriptor takes the path of the file and returns the SecurityDescriptor for the file.
// This needs admin permissions or SeBackupPrivilege for getting the full SD.
// If there are no admin permissions, only the current user's owner, group and DACL will be got.
func GetSecurityDescriptor(filePath string) (securityDescriptor *[]byte, err error) {
	onceBackup.Do(enableBackupPrivilege)

	var sd *windows.SECURITY_DESCRIPTOR

	if lowerPrivileges.Load() {
		sd, err = getNamedSecurityInfoLow(filePath)
	} else {
		sd, err = getNamedSecurityInfoHigh(filePath)
	}
	if err != nil {
		if !lowerPrivileges.Load() && isHandlePrivilegeNotHeldError(err) {
			// If ERROR_PRIVILEGE_NOT_HELD is encountered, fallback to backups/restores using lower non-admin privileges.
			lowerPrivileges.Store(true)
			sd, err = getNamedSecurityInfoLow(filePath)
			if err != nil {
				return nil, fmt.Errorf("get low-level named security info failed with: %w", err)
			}
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

	if lowerPrivileges.Load() {
		err = setNamedSecurityInfoLow(filePath, dacl)
	} else {
		err = setNamedSecurityInfoHigh(filePath, owner, group, dacl, sacl)
	}

	if err != nil {
		if !lowerPrivileges.Load() && isHandlePrivilegeNotHeldError(err) {
			// If ERROR_PRIVILEGE_NOT_HELD is encountered, fallback to backups/restores using lower non-admin privileges.
			lowerPrivileges.Store(true)
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

// enableBackupPrivilege enables privilege for backing up security descriptors
func enableBackupPrivilege() {
	err := enableProcessPrivileges([]string{SeBackupPrivilege})
	if err != nil {
		debug.Log("error enabling backup privilege: %v", err)
	}
}

// enableBackupPrivilege enables privilege for restoring security descriptors
func enableRestorePrivilege() {
	err := enableProcessPrivileges([]string{SeRestorePrivilege, SeSecurityPrivilege, SeTakeOwnershipPrivilege})
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

// The code below was adapted from
// https://github.com/microsoft/go-winio/blob/3c9576c9346a1892dee136329e7e15309e82fb4f/privilege.go
// under MIT license.

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
	ERROR_NOT_ALL_ASSIGNED windows.Errno = windows.ERROR_NOT_ALL_ASSIGNED
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
		debug.Log("Not all requested privileges were fully set: %v. AdjustTokenPrivileges returned warning: %v", privileges, err)
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

// The functions below are copied over from https://github.com/microsoft/go-winio/blob/main/zsyscall_windows.go under MIT license.

// This windows api always returns an error even in case of success, warnings (partial success) and error cases.
//
// Full success - When we call this with admin permissions, it returns DNS_ERROR_RCODE_NO_ERROR (0).
// This gets translated to errErrorEinval and ultimately in adjustTokenPrivileges, it gets ignored.
//
// Partial success - If we call this api without admin privileges, privileges related to SACLs do not get set and
// though the api returns success, it returns an error - golang.org/x/sys/windows.ERROR_NOT_ALL_ASSIGNED (1300)
func adjustTokenPrivileges(token windows.Token, releaseAll bool, input *byte, outputSize uint32, output *byte, requiredSize *uint32) (success bool, err error) {
	var _p0 uint32
	if releaseAll {
		_p0 = 1
	}
	r0, _, e1 := syscall.SyscallN(procAdjustTokenPrivileges.Addr(), uintptr(token), uintptr(_p0), uintptr(unsafe.Pointer(input)), uintptr(outputSize), uintptr(unsafe.Pointer(output)), uintptr(unsafe.Pointer(requiredSize)))
	success = r0 != 0
	if true {
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
	r1, _, e1 := syscall.SyscallN(procLookupPrivilegeDisplayNameW.Addr(), uintptr(unsafe.Pointer(systemName)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(buffer)), uintptr(unsafe.Pointer(size)), uintptr(unsafe.Pointer(languageID)))
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
	r1, _, e1 := syscall.SyscallN(procLookupPrivilegeNameW.Addr(), uintptr(unsafe.Pointer(systemName)), uintptr(unsafe.Pointer(luid)), uintptr(unsafe.Pointer(buffer)), uintptr(unsafe.Pointer(size)))
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

// The code below was copied from https://github.com/microsoft/go-winio/blob/main/tools/mkwinsyscall/mkwinsyscall.go under MIT license.

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return errErrorEinval
	case errnoErrorIOPending:
		return errErrorIOPending
	}
	return e
}
