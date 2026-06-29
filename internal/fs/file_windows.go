package fs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/restic/restic/internal/data"
	"golang.org/x/sys/windows"
)

// fixpath returns an absolute path on windows, so restic can open long file
// names.
func fixpath(name string) string {
	abspath, err := filepath.Abs(name)
	if err == nil {
		// Check if \\?\UNC\ already exist
		if strings.HasPrefix(abspath, uncPathPrefix) {
			return abspath
		}
		// Check if \\?\GLOBALROOT exists which marks volume shadow copy snapshots
		if strings.HasPrefix(abspath, globalRootPrefix) {
			if strings.Count(abspath, `\`) == 5 {
				// Append slash if this just a volume name, e.g. `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopyXX`
				// Without the trailing slash any access to the volume itself will fail.
				return abspath + string(filepath.Separator)
			}
			return abspath
		}
		// Check if \\?\ already exist
		if strings.HasPrefix(abspath, extendedPathPrefix) {
			return abspath
		}
		// Check if path starts with \\
		if strings.HasPrefix(abspath, `\\`) {
			return strings.Replace(abspath, `\\`, uncPathPrefix, 1)
		}
		// Normal path
		return extendedPathPrefix + abspath
	}
	return name
}

// Chmod changes the mode of the named file to mode.
func chmod(name string, mode os.FileMode) error {
	return os.Chmod(fixpath(name), mode)
}

// clearSystem removes the system attribute from the file.
func clearSystem(path string) error {
	return clearAttribute(path, windows.FILE_ATTRIBUTE_SYSTEM)
}

// clearAttribute removes the specified attribute from the file.
func clearAttribute(path string, attribute uint32) error {
	ptr, err := windows.UTF16PtrFromString(fixpath(path))
	if err != nil {
		return err
	}
	fileAttributes, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return err
	}
	if fileAttributes&attribute != 0 {
		// Clear the attribute
		fileAttributes &= ^attribute
		err = windows.SetFileAttributes(ptr, fileAttributes)
		if err != nil {
			return err
		}
	}
	return nil
}

// openHandleForEA return a file handle for file or dir for setting/getting EAs
func openHandleForEA(nodeType data.NodeType, path string, writeAccess bool) (handle windows.Handle, err error) {
	path = fixpath(path)
	fileAccess := windows.FILE_READ_EA
	if writeAccess {
		fileAccess = fileAccess | windows.FILE_WRITE_EA
	}

	switch nodeType {
	case data.NodeTypeFile, data.NodeTypeDir:
		utf16Path := windows.StringToUTF16Ptr(path)
		handle, err = windows.CreateFile(utf16Path, uint32(fileAccess), 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	default:
		return 0, nil
	}
	return handle, err
}
