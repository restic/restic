//go:build windows
// +build windows

package restic

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32dll      = syscall.NewLazyDLL("kernel32.dll")
	findFirstStreamW = kernel32dll.NewProc("FindFirstStreamW")
	findNextStreamW  = kernel32dll.NewProc("FindNextStreamW")
	findClose        = kernel32dll.NewProc("FindClose")
)

type (
	HANDLE uintptr
)

const (
	maxPath                 = 296
	streamInfoLevelStandard = 0
	invalidFileHandle       = ^HANDLE(0)
)

type Win32FindStreamData struct {
	size int64
	name [maxPath]uint16
}

/*
	HANDLE WINAPI FindFirstStreamW(
	__in        LPCWSTR lpFileName,
	__in        STREAM_INFO_LEVELS InfoLevel, (0 standard, 1 max infos)
	__out       LPVOID lpFindStreamData, (return information about file in a WIN32_FIND_STREAM_DATA if 0 is given in infos_level
	__reserved  DWORD dwFlags (Reserved for future use. This parameter must be zero.) cf: doc
	);
	https://msdn.microsoft.com/en-us/library/aa364424(v=vs.85).aspx
*/
// GetADStreamNames returns the ads stream names for the passed fileName.
// If success is true, it means ADS files were found.
func GetADStreamNames(fileName string) (success bool, streamNames []string, err error) {
	h, success, firstname, err := findFirstStream(fileName)
	defer closeHandle(h)
	if success {
		if !strings.Contains(firstname, "::") {
			//If fileName is a directory which has ADS, the ADS name comes in the first stream itself between the two :
			//file ads firstname comes as ::$DATA
			streamNames = append(streamNames, firstname)
		}
		for {
			endStream, name, err2 := findNextStream(h)
			err = err2
			if endStream {
				break
			}
			streamNames = append(streamNames, name)
		}
	}
	// If the handle is found successfully, success is true, but the windows api
	// still returns an error object. It doesn't mean that an error occurred.
	if isHandleEOFError(err) {
		// This error is expected, we don't need to expose it.
		err = nil
	}
	return success, streamNames, err
}

// findFirstStream gets the handle and stream type for the first stream
// If the handle is found successfully, success is true, but the windows api
// still returns an error object. It doesn't mean that an error occurred.
func findFirstStream(fileName string) (handle HANDLE, success bool, streamType string, err error) {
	fsd := &Win32FindStreamData{}

	ptr, err := syscall.UTF16PtrFromString(fileName)
	if err != nil {
		return invalidFileHandle, false, "<nil>", err
	}
	ret, _, err := findFirstStreamW.Call(
		uintptr(unsafe.Pointer(ptr)),
		streamInfoLevelStandard,
		uintptr(unsafe.Pointer(fsd)),
		0,
	)
	h := HANDLE(ret)

	streamType = windows.UTF16ToString(fsd.name[:])
	return h, h != invalidFileHandle, streamType, err
}

// findNextStream finds the next ads stream name
// endStream indicites if this is the last stream, name is the stream name.
// err being returned does not mean an error occurred.
func findNextStream(handle HANDLE) (endStream bool, name string, err error) {
	fsd := &Win32FindStreamData{}
	ret, _, err := findNextStreamW.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(fsd)),
	)
	name = windows.UTF16ToString(fsd.name[:])
	return ret != 1, name, err
}

// closeHandle closes the passed handle
func closeHandle(handle HANDLE) bool {
	ret, _, _ := findClose.Call(
		uintptr(handle),
	)
	return ret != 0
}

// TrimAds trims the ads file part from the passed filename and returns the base name.
func TrimAds(str string) string {
	dir, filename := filepath.Split(str)
	if strings.Contains(filename, ":") {
		out := filepath.Join(dir, strings.Split(filename, ":")[0])
		return out
	} else {
		return str
	}
}

// IsAds checks if the passed file name is an ads file.
func IsAds(str string) bool {
	filename := filepath.Base(str)
	// Only ADS filenames can contain ":" in windows.
	return strings.Contains(filename, ":")
}

// isHandleEOFError checks if the error is ERROR_HANDLE_EOF
func isHandleEOFError(err error) bool {
	// Use a type assertion to check if the error is of type syscall.Errno
	if errno, ok := err.(syscall.Errno); ok {
		// Compare the error code to the expected value
		return errno == syscall.ERROR_HANDLE_EOF
	}
	return false
}
