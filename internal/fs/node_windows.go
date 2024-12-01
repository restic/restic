package fs

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"unsafe"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sys/windows"
)

var (
	modAdvapi32     = syscall.NewLazyDLL("advapi32.dll")
	procEncryptFile = modAdvapi32.NewProc("EncryptFileW")
	procDecryptFile = modAdvapi32.NewProc("DecryptFileW")
)

const (
	extendedPathPrefix = `\\?\`
	uncPathPrefix      = `\\?\UNC\`
	globalRootPrefix   = `\\?\GLOBALROOT\`
	volumeGUIDPrefix   = `\\?\Volume{`
)

// mknod is not supported on Windows.
func mknod(_ string, _ uint32, _ uint64) (err error) {
	return errors.New("device nodes cannot be created on windows")
}

// Windows doesn't need lchown
func lchown(_ string, _ int, _ int) (err error) {
	return nil
}

// utimesNano is like syscall.UtimesNano, except that it sets FILE_FLAG_OPEN_REPARSE_POINT.
func utimesNano(path string, atime, mtime int64, _ restic.NodeType) error {
	// tweaked version of UtimesNano from go/src/syscall/syscall_windows.go
	pathp, e := syscall.UTF16PtrFromString(fixpath(path))
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

	a := syscall.NsecToFiletime(atime)
	w := syscall.NsecToFiletime(mtime)
	return syscall.SetFileTime(h, nil, &a, &w)
}

// restore extended attributes for windows
func nodeRestoreExtendedAttributes(node *restic.Node, path string) (err error) {
	count := len(node.ExtendedAttributes)
	if count > 0 {
		eas := make([]extendedAttribute, count)
		for i, attr := range node.ExtendedAttributes {
			eas[i] = extendedAttribute{Name: attr.Name, Value: attr.Value}
		}
		if errExt := restoreExtendedAttributes(node.Type, path, eas); errExt != nil {
			return errExt
		}
	}
	return nil
}

// fill extended attributes in the node
func nodeFillExtendedAttributes(node *restic.Node, meta metadataHandle, _ bool) (err error) {
	if strings.Contains(filepath.Base(meta.Name()), ":") {
		// Do not process for Alternate Data Streams in Windows
		return nil
	}
	// only capture xattrs for file/dir
	if node.Type != restic.NodeTypeFile && node.Type != restic.NodeTypeDir {
		return nil
	}

	node.ExtendedAttributes, err = meta.Xattr(false)
	return err
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
func restoreExtendedAttributes(nodeType restic.NodeType, path string, eas []extendedAttribute) (err error) {
	// only restore xattrs for file/dir
	if nodeType != restic.NodeTypeFile && nodeType != restic.NodeTypeDir {
		return nil
	}

	var fileHandle windows.Handle
	if fileHandle, err = openWriteHandleForEA(path); err != nil {
		return errors.Errorf("set EA failed while opening file handle for path %v, with: %v", path, err)
	}
	defer closeFileHandle(fileHandle, path) // Replaced inline defer with named function call

	// clear old unexpected xattrs by setting them to an empty value
	oldEAs, err := fgetEA(fileHandle)
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
			eas = append(eas, extendedAttribute{Name: oldEA.Name, Value: nil})
		}
	}

	if err = fsetEA(fileHandle, eas); err != nil {
		return errors.Errorf("set EA failed for path %v, with: %v", path, err)
	}
	return nil
}

// restoreGenericAttributes restores generic attributes for Windows
func nodeRestoreGenericAttributes(node *restic.Node, path string, warn func(msg string)) (err error) {
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
		if err := setSecurityDescriptor(path, windowsAttributes.SecurityDescriptor); err != nil {
			errs = append(errs, fmt.Errorf("error restoring security descriptor for: %s : %v", path, err))
		}
	}

	restic.HandleUnknownGenericAttributesFound(unknownAttribs, warn)
	return errors.Join(errs...)
}

// genericAttributesToWindowsAttrs converts the generic attributes map to a WindowsAttributes and also returns a string of unknown attributes that it could not convert.
func genericAttributesToWindowsAttrs(attrs map[restic.GenericAttributeType]json.RawMessage) (windowsAttributes restic.WindowsAttributes, unknownAttribs []restic.GenericAttributeType, err error) {
	waValue := reflect.ValueOf(&windowsAttributes).Elem()
	unknownAttribs, err = restic.GenericAttributesToOSAttrs(attrs, reflect.TypeOf(windowsAttributes), &waValue, "windows")
	return windowsAttributes, unknownAttribs, err
}

// restoreCreationTime gets the creation time from the data and sets it to the file/folder at
// the specified path.
func restoreCreationTime(path string, creationTime *syscall.Filetime) (err error) {
	pathPointer, err := syscall.UTF16PtrFromString(fixpath(path))
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
	pathPointer, err := syscall.UTF16PtrFromString(fixpath(path))
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
			if IsAccessDenied(err) || errors.Is(err, windows.ERROR_FILE_READ_ONLY) {
				// If existing file already has readonly or system flag, encrypt file call fails.
				// The readonly and system flags will be set again at the end of this func if they are needed.
				err = ResetPermissions(path)
				if err != nil {
					return fmt.Errorf("failed to encrypt file: failed to reset permissions: %s : %v", path, err)
				}
				err = clearSystem(path)
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
				if IsAccessDenied(err) || errors.Is(err, windows.ERROR_FILE_READ_ONLY) {
					// If existing file already has readonly or system flag, decrypt file call fails.
					// The readonly and system flags will be set again after this func if they are needed.
					err = ResetPermissions(path)
					if err != nil {
						return fmt.Errorf("failed to encrypt file: failed to reset permissions: %s : %v", path, err)
					}
					err = clearSystem(path)
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

// nodeFillGenericAttributes fills in the generic attributes for windows like File Attributes,
// Created time and Security Descriptors.
func nodeFillGenericAttributes(node *restic.Node, meta metadataHandle, stat *ExtendedFileInfo) error {
	path := meta.Name()
	if strings.Contains(filepath.Base(path), ":") {
		// Do not process for Alternate Data Streams in Windows
		return nil
	}

	isVolume, err := isVolumePath(path)
	if err != nil {
		return err
	}
	if isVolume {
		// Do not process file attributes, created time and sd for windows root volume paths
		// Security descriptors are not supported for root volume paths.
		// Though file attributes and created time are supported for root volume paths,
		// we ignore them and we do not want to replace them during every restore.
		return nil
	}

	var sd *[]byte
	if node.Type == restic.NodeTypeFile || node.Type == restic.NodeTypeDir {
		if sd, err = meta.SecurityDescriptor(); err != nil {
			return err
		}
	}

	winFI := stat.sys.(*syscall.Win32FileAttributeData)

	// Add Windows attributes
	node.GenericAttributes, err = restic.WindowsAttrsToGenericAttributes(restic.WindowsAttributes{
		CreationTime:       &winFI.CreationTime,
		FileAttributes:     &winFI.FileAttributes,
		SecurityDescriptor: sd,
	})
	return err
}

// isVolumePath returns whether a path refers to a volume
func isVolumePath(path string) (bool, error) {
	volName, err := prepareVolumeName(path)
	if err != nil {
		return false, err
	}

	cleanPath := filepath.Clean(path)
	cleanVolume := filepath.Clean(volName + `\`)
	return cleanPath == cleanVolume, nil
}

// prepareVolumeName prepares the volume name for different cases in Windows
func prepareVolumeName(path string) (volumeName string, err error) {
	// Check if it's an extended length path
	if strings.HasPrefix(path, globalRootPrefix) {
		// Extract the VSS snapshot volume name eg. `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopyXX`
		if parts := strings.SplitN(path, `\`, 7); len(parts) >= 6 {
			volumeName = strings.Join(parts[:6], `\`)
		} else {
			volumeName = filepath.VolumeName(path)
		}
	} else {
		if !strings.HasPrefix(path, volumeGUIDPrefix) { // Handle volume GUID path
			if strings.HasPrefix(path, uncPathPrefix) {
				// Convert \\?\UNC\ extended path to standard path to get the volume name correctly
				path = `\\` + path[len(uncPathPrefix):]
			} else if strings.HasPrefix(path, extendedPathPrefix) {
				//Extended length path prefix needs to be trimmed to get the volume name correctly
				path = path[len(extendedPathPrefix):]
			} else {
				// Use the absolute path
				path, err = filepath.Abs(path)
				if err != nil {
					return "", fmt.Errorf("failed to get absolute path: %w", err)
				}
			}
		}
		volumeName = filepath.VolumeName(path)
	}
	return volumeName, nil
}
