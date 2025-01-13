//go:build windows
// +build windows

package fs

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// extendedStat extracts info into an ExtendedFileInfo for Windows.
func extendedStat(fi os.FileInfo) *ExtendedFileInfo {
	s, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		panic(fmt.Sprintf("conversion to syscall.Win32FileAttributeData failed, type is %T", fi.Sys()))
	}

	extFI := ExtendedFileInfo{
		Name: fi.Name(),
		Mode: fi.Mode(),

		Size: int64(s.FileSizeLow) | (int64(s.FileSizeHigh) << 32),
		sys:  fi.Sys(),
	}

	atime := syscall.NsecToTimespec(s.LastAccessTime.Nanoseconds())
	extFI.AccessTime = time.Unix(atime.Unix())

	mtime := syscall.NsecToTimespec(s.LastWriteTime.Nanoseconds())
	extFI.ModTime = time.Unix(mtime.Unix())

	// Windows does not have the concept of a "change time" in the sense Unix uses it, so we're using the LastWriteTime here.
	extFI.ChangeTime = extFI.ModTime

	return &extFI
}

// RecallOnDataAccess checks if a file is available locally on the disk or if the file is
// just a placeholder which must be downloaded from a remote server. This is typically used
// in cloud syncing services (e.g. OneDrive) to prevent downloading files from cloud storage
// until they are accessed.
func (fi *ExtendedFileInfo) RecallOnDataAccess() (bool, error) {
	attrs, ok := fi.sys.(*syscall.Win32FileAttributeData)
	if !ok {
		return false, fmt.Errorf("could not determine file attributes: %s", fi.Name)
	}

	if attrs.FileAttributes&windows.FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS > 0 {
		return true, nil
	}

	return false, nil
}
