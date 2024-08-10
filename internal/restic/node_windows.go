package restic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"golang.org/x/sys/windows"
)

// WindowsAttributes are the genericAttributes for Windows OS
type WindowsAttributes struct {
	// CreationTime is used for storing creation time for windows files.
	CreationTime *syscall.Filetime `generic:"creation_time"`
	// FileAttributes is used for storing file attributes for windows files.
	FileAttributes *uint32 `generic:"file_attributes"`
	// SecurityDescriptor is used for storing security descriptors which includes
	// owner, group, discretionary access control list (DACL), system access control list (SACL)
	SecurityDescriptor *[]byte `generic:"security_descriptor"`
}

var (
	modAdvapi32     = syscall.NewLazyDLL("advapi32.dll")
	procEncryptFile = modAdvapi32.NewProc("EncryptFileW")
	procDecryptFile = modAdvapi32.NewProc("DecryptFileW")

	// eaSupportedVolumesMap is a map of volumes to boolean values indicating if they support extended attributes.
	eaSupportedVolumesMap = sync.Map{}
)

const (
	extendedPathPrefix = `\\?\`
	uncPathPrefix      = `\\?\UNC\`
	globalRootPrefix   = `\\?\GLOBALROOT\`
)

// mknod is not supported on Windows.
func mknod(_ string, _ uint32, _ uint64) (err error) {
	return errors.New("device nodes cannot be created on windows")
}

// Windows doesn't need lchown
func lchown(_ string, _ int, _ int) (err error) {
	return nil
}

// restoreSymlinkTimestamps restores timestamps for symlinks
func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	// tweaked version of UtimesNano from go/src/syscall/syscall_windows.go
	pathp, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	h, e := syscall.CreateFile(pathp,
		syscall.FILE_WRITE_ATTRIBUTES, syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return e
	}

	defer func() {
		err := syscall.Close(h)
		if err != nil {
			debug.Log("Error closing file handle for %s: %v\n", path, err)
		}
	}()

	a := syscall.NsecToFiletime(syscall.TimespecToNsec(utimes[0]))
	w := syscall.NsecToFiletime(syscall.TimespecToNsec(utimes[1]))
	return syscall.SetFileTime(h, nil, &a, &w)
}

// restore extended attributes for windows
func (node Node) restoreExtendedAttributes(path string) (err error) {
	count := len(node.ExtendedAttributes)
	if count > 0 {
		eas := make([]fs.ExtendedAttribute, count)
		for i, attr := range node.ExtendedAttributes {
			eas[i] = fs.ExtendedAttribute{Name: attr.Name, Value: attr.Value}
		}
		if errExt := restoreExtendedAttributes(node.Type, path, eas); errExt != nil {
			return errExt
		}
	}
	return nil
}

// fill extended attributes in the node. This also includes the Generic attributes for windows.
func (node *Node) fillExtendedAttributes(path string, _ bool) (err error) {
	var fileHandle windows.Handle
	if fileHandle, err = fs.OpenHandleForEA(node.Type, path, false); fileHandle == 0 {
		return nil
	}
	if err != nil {
		return errors.Errorf("get EA failed while opening file handle for path %v, with: %v", path, err)
	}
	defer closeFileHandle(fileHandle, path) // Replaced inline defer with named function call
	//Get the windows Extended Attributes using the file handle
	var extAtts []fs.ExtendedAttribute
	extAtts, err = fs.GetFileEA(fileHandle)
	debug.Log("fillExtendedAttributes(%v) %v", path, extAtts)
	if err != nil {
		return errors.Errorf("get EA failed for path %v, with: %v", path, err)
	}
	if len(extAtts) == 0 {
		return nil
	}

	//Fill the ExtendedAttributes in the node using the name/value pairs in the windows EA
	for _, attr := range extAtts {
		extendedAttr := ExtendedAttribute{
			Name:  attr.Name,
			Value: attr.Value,
		}

		node.ExtendedAttributes = append(node.ExtendedAttributes, extendedAttr)
	}
	return nil
}

// closeFileHandle safely closes a file handle and logs any errors.
func closeFileHandle(fileHandle windows.Handle, path string) {
	err := windows.CloseHandle(fileHandle)
	if err != nil {
		debug.Log("Error closing file handle for %s: %v\n", path, err)
	}
}

// restoreExtendedAttributes handles restore of the Windows Extended Attributes to the specified path.
// The Windows API requires setting of all the Extended Attributes in one call.
func restoreExtendedAttributes(nodeType, path string, eas []fs.ExtendedAttribute) (err error) {
	var fileHandle windows.Handle
	if fileHandle, err = fs.OpenHandleForEA(nodeType, path, true); fileHandle == 0 {
		return nil
	}
	if err != nil {
		return errors.Errorf("set EA failed while opening file handle for path %v, with: %v", path, err)
	}
	defer closeFileHandle(fileHandle, path) // Replaced inline defer with named function call

	// clear old unexpected xattrs by setting them to an empty value
	oldEAs, err := fs.GetFileEA(fileHandle)
	if err != nil {
		return err
	}

	for _, oldEA := range oldEAs {
		found := false
		for _, ea := range eas {
			if strings.EqualFold(ea.Name, oldEA.Name) {
				found = true
				break
			}
		}

		if !found {
			eas = append(eas, fs.ExtendedAttribute{Name: oldEA.Name, Value: nil})
		}
	}

	if err = fs.SetFileEA(fileHandle, eas); err != nil {
		return errors.Errorf("set EA failed for path %v, with: %v", path, err)
	}
	return nil
}

type statT syscall.Win32FileAttributeData

func toStatT(i interface{}) (*statT, bool) {
	s, ok := i.(*syscall.Win32FileAttributeData)
	if ok && s != nil {
		return (*statT)(s), true
	}
	return nil, false
}

func (s statT) dev() uint64   { return 0 }
func (s statT) ino() uint64   { return 0 }
func (s statT) nlink() uint64 { return 0 }
func (s statT) uid() uint32   { return 0 }
func (s statT) gid() uint32   { return 0 }
func (s statT) rdev() uint64  { return 0 }

func (s statT) size() int64 {
	return int64(s.FileSizeLow) | (int64(s.FileSizeHigh) << 32)
}

func (s statT) atim() syscall.Timespec {
	return syscall.NsecToTimespec(s.LastAccessTime.Nanoseconds())
}

func (s statT) mtim() syscall.Timespec {
	return syscall.NsecToTimespec(s.LastWriteTime.Nanoseconds())
}

func (s statT) ctim() syscall.Timespec {
	// Windows does not have the concept of a "change time" in the sense Unix uses it, so we're using the LastWriteTime here.
	return s.mtim()
}

// restoreGenericAttributes restores generic attributes for Windows
func (node Node) restoreGenericAttributes(path string, warn func(msg string)) (err error) {
	if len(node.GenericAttributes) == 0 {
		return nil
	}
	var errs []error
	windowsAttributes, unknownAttribs, err := genericAttributesToWindowsAttrs(node.GenericAttributes)
	if err != nil {
		return fmt.Errorf("error parsing generic attribute for: %s : %v", path, err)
	}
	if windowsAttributes.CreationTime != nil {
		if err := restoreCreationTime(path, windowsAttributes.CreationTime); err != nil {
			errs = append(errs, fmt.Errorf("error restoring creation time for: %s : %v", path, err))
		}
	}
	if windowsAttributes.FileAttributes != nil {
		if err := restoreFileAttributes(path, windowsAttributes.FileAttributes); err != nil {
			errs = append(errs, fmt.Errorf("error restoring file attributes for: %s : %v", path, err))
		}
	}
	if windowsAttributes.SecurityDescriptor != nil {
		if err := fs.SetSecurityDescriptor(path, windowsAttributes.SecurityDescriptor); err != nil {
			errs = append(errs, fmt.Errorf("error restoring security descriptor for: %s : %v", path, err))
		}
	}

	HandleUnknownGenericAttributesFound(unknownAttribs, warn)
	return errors.CombineErrors(errs...)
}

// genericAttributesToWindowsAttrs converts the generic attributes map to a WindowsAttributes and also returns a string of unknown attributes that it could not convert.
func genericAttributesToWindowsAttrs(attrs map[GenericAttributeType]json.RawMessage) (windowsAttributes WindowsAttributes, unknownAttribs []GenericAttributeType, err error) {
	waValue := reflect.ValueOf(&windowsAttributes).Elem()
	unknownAttribs, err = genericAttributesToOSAttrs(attrs, reflect.TypeOf(windowsAttributes), &waValue, "windows")
	return windowsAttributes, unknownAttribs, err
}

// restoreCreationTime gets the creation time from the data and sets it to the file/folder at
// the specified path.
func restoreCreationTime(path string, creationTime *syscall.Filetime) (err error) {
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(pathPointer,
		syscall.FILE_WRITE_ATTRIBUTES, syscall.FILE_SHARE_WRITE, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer func() {
		if err := syscall.Close(handle); err != nil {
			debug.Log("Error closing file handle for %s: %v\n", path, err)
		}
	}()
	return syscall.SetFileTime(handle, creationTime, nil, nil)
}

// restoreFileAttributes gets the File Attributes from the data and sets them to the file/folder
// at the specified path.
func restoreFileAttributes(path string, fileAttributes *uint32) (err error) {
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	err = fixEncryptionAttribute(path, fileAttributes, pathPointer)
	if err != nil {
		debug.Log("Could not change encryption attribute for path: %s: %v", path, err)
	}
	return syscall.SetFileAttributes(pathPointer, *fileAttributes)
}

// fixEncryptionAttribute checks if a file needs to be marked encrypted and is not already encrypted, it sets
// the FILE_ATTRIBUTE_ENCRYPTED. Conversely, if the file needs to be marked unencrypted and it is already
// marked encrypted, it removes the FILE_ATTRIBUTE_ENCRYPTED.
func fixEncryptionAttribute(path string, attrs *uint32, pathPointer *uint16) (err error) {
	if *attrs&windows.FILE_ATTRIBUTE_ENCRYPTED != 0 {
		// File should be encrypted.
		err = encryptFile(pathPointer)
		if err != nil {
			if fs.IsAccessDenied(err) || errors.Is(err, windows.ERROR_FILE_READ_ONLY) {
				// If existing file already has readonly or system flag, encrypt file call fails.
				// The readonly and system flags will be set again at the end of this func if they are needed.
				err = fs.ResetPermissions(path)
				if err != nil {
					return fmt.Errorf("failed to encrypt file: failed to reset permissions: %s : %v", path, err)
				}
				err = fs.ClearSystem(path)
				if err != nil {
					return fmt.Errorf("failed to encrypt file: failed to clear system flag: %s : %v", path, err)
				}
				err = encryptFile(pathPointer)
				if err != nil {
					return fmt.Errorf("failed retry to encrypt file: %s : %v", path, err)
				}
			} else {
				return fmt.Errorf("failed to encrypt file: %s : %v", path, err)
			}
		}
	} else {
		existingAttrs, err := windows.GetFileAttributes(pathPointer)
		if err != nil {
			return fmt.Errorf("failed to get file attributes for existing file: %s : %v", path, err)
		}
		if existingAttrs&windows.FILE_ATTRIBUTE_ENCRYPTED != 0 {
			// File should not be encrypted, but its already encrypted. Decrypt it.
			err = decryptFile(pathPointer)
			if err != nil {
				if fs.IsAccessDenied(err) || errors.Is(err, windows.ERROR_FILE_READ_ONLY) {
					// If existing file already has readonly or system flag, decrypt file call fails.
					// The readonly and system flags will be set again after this func if they are needed.
					err = fs.ResetPermissions(path)
					if err != nil {
						return fmt.Errorf("failed to encrypt file: failed to reset permissions: %s : %v", path, err)
					}
					err = fs.ClearSystem(path)
					if err != nil {
						return fmt.Errorf("failed to decrypt file: failed to clear system flag: %s : %v", path, err)
					}
					err = decryptFile(pathPointer)
					if err != nil {
						return fmt.Errorf("failed retry to decrypt file: %s : %v", path, err)
					}
				} else {
					return fmt.Errorf("failed to decrypt file: %s : %v", path, err)
				}
			}
		}
	}
	return err
}

// encryptFile set the encrypted flag on the file.
func encryptFile(pathPointer *uint16) error {
	// Call EncryptFile function
	ret, _, err := procEncryptFile.Call(uintptr(unsafe.Pointer(pathPointer)))
	if ret == 0 {
		return err
	}
	return nil
}

// decryptFile removes the encrypted flag from the file.
func decryptFile(pathPointer *uint16) error {
	// Call DecryptFile function
	ret, _, err := procDecryptFile.Call(uintptr(unsafe.Pointer(pathPointer)))
	if ret == 0 {
		return err
	}
	return nil
}

// fillGenericAttributes fills in the generic attributes for windows like File Attributes,
// Created time and Security Descriptors.
// It also checks if the volume supports extended attributes and stores the result in a map
// so that it does not have to be checked again for subsequent calls for paths in the same volume.
func (node *Node) fillGenericAttributes(path string, fi os.FileInfo, stat *statT) (allowExtended bool, err error) {
	if strings.Contains(filepath.Base(path), ":") {
		// Do not process for Alternate Data Streams in Windows
		// Also do not allow processing of extended attributes for ADS.
		return false, nil
	}

	if strings.HasSuffix(filepath.Clean(path), `\`) {
		// filepath.Clean(path) ends with '\' for Windows root volume paths only
		// Do not process file attributes, created time and sd for windows root volume paths
		// Security descriptors are not supported for root volume paths.
		// Though file attributes and created time are supported for root volume paths,
		// we ignore them and we do not want to replace them during every restore.
		allowExtended, err = checkAndStoreEASupport(path)
		if err != nil {
			return false, err
		}
		return allowExtended, nil
	}

	var sd *[]byte
	if node.Type == "file" || node.Type == "dir" {
		// Check EA support and get security descriptor for file/dir only
		allowExtended, err = checkAndStoreEASupport(path)
		if err != nil {
			return false, err
		}
		if sd, err = fs.GetSecurityDescriptor(path); err != nil {
			return allowExtended, err
		}
	}
	// Add Windows attributes
	node.GenericAttributes, err = WindowsAttrsToGenericAttributes(WindowsAttributes{
		CreationTime:       getCreationTime(fi, path),
		FileAttributes:     &stat.FileAttributes,
		SecurityDescriptor: sd,
	})
	return allowExtended, err
}

// checkAndStoreEASupport checks if the volume of the path supports extended attributes and stores the result in a map
// If the result is already in the map, it returns the result from the map.
func checkAndStoreEASupport(path string) (isEASupportedVolume bool, err error) {
	// Check if it's an extended length path
	if strings.HasPrefix(path, uncPathPrefix) {
		// Convert \\?\UNC\ extended path to standard path to get the volume name correctly
		path = `\\` + path[len(uncPathPrefix):]
	} else if strings.HasPrefix(path, extendedPathPrefix) {
		//Extended length path prefix needs to be trimmed to get the volume name correctly
		path = path[len(extendedPathPrefix):]
	} else if strings.HasPrefix(path, globalRootPrefix) {
		// EAs are not supported for \\?\GLOBALROOT i.e. VSS snapshots
		return false, nil
	} else {
		// Use the absolute path
		path, err = filepath.Abs(path)
		if err != nil {
			return false, fmt.Errorf("failed to get absolute path: %w", err)
		}
	}
	volumeName := filepath.VolumeName(path)
	if volumeName == "" {
		return false, nil
	}
	eaSupportedValue, exists := eaSupportedVolumesMap.Load(volumeName)
	if exists {
		return eaSupportedValue.(bool), nil
	}

	// Add backslash to the volume name to ensure it is a valid path
	isEASupportedVolume, err = fs.PathSupportsExtendedAttributes(volumeName + `\`)
	if err == nil {
		eaSupportedVolumesMap.Store(volumeName, isEASupportedVolume)
	}
	return isEASupportedVolume, err
}

// windowsAttrsToGenericAttributes converts the WindowsAttributes to a generic attributes map using reflection
func WindowsAttrsToGenericAttributes(windowsAttributes WindowsAttributes) (attrs map[GenericAttributeType]json.RawMessage, err error) {
	// Get the value of the WindowsAttributes
	windowsAttributesValue := reflect.ValueOf(windowsAttributes)
	return osAttrsToGenericAttributes(reflect.TypeOf(windowsAttributes), &windowsAttributesValue, runtime.GOOS)
}

// getCreationTime gets the value for the WindowsAttribute CreationTime in a windows specific time format.
// The value is a 64-bit value representing the number of 100-nanosecond intervals since January 1, 1601 (UTC)
// split into two 32-bit parts: the low-order DWORD and the high-order DWORD for efficiency and interoperability.
// The low-order DWORD represents the number of 100-nanosecond intervals elapsed since January 1, 1601, modulo
// 2^32. The high-order DWORD represents the number of times the low-order DWORD has overflowed.
func getCreationTime(fi os.FileInfo, path string) (creationTimeAttribute *syscall.Filetime) {
	attrib, success := fi.Sys().(*syscall.Win32FileAttributeData)
	if success && attrib != nil {
		return &attrib.CreationTime
	} else {
		debug.Log("Could not get create time for path: %s", path)
		return nil
	}
}
