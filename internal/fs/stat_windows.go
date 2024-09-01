//go:build windows
// +build windows

package fs

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// extendedStat extracts info into an ExtendedFileInfo for Windows.
func extendedStat(fi os.FileInfo) ExtendedFileInfo {
	s, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		panic(fmt.Sprintf("conversion to syscall.Win32FileAttributeData failed, type is %T", fi.Sys()))
	}

	extFI := ExtendedFileInfo{
		FileInfo: fi,
		Size:     int64(s.FileSizeLow) | (int64(s.FileSizeHigh) << 32),
	}

	atime := syscall.NsecToTimespec(s.LastAccessTime.Nanoseconds())
	extFI.AccessTime = time.Unix(atime.Unix())

	mtime := syscall.NsecToTimespec(s.LastWriteTime.Nanoseconds())
	extFI.ModTime = time.Unix(mtime.Unix())

	// Windows does not have the concept of a "change time" in the sense Unix uses it, so we're using the LastWriteTime here.
	extFI.ChangeTime = extFI.ModTime

	return extFI
}
