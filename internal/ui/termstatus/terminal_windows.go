//go:build windows
// +build windows

package termstatus

import (
	"io"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// clearCurrentLine removes all characters from the current line and resets the
// cursor position to the first column.
func clearCurrentLine(fd uintptr) func(io.Writer, uintptr) {
	// easy case, the terminal is cmd or psh, without redirection
	if isWindowsTerminal(fd) {
		return windowsClearCurrentLine
	}

	// assume we're running in mintty/cygwin
	return posixClearCurrentLine
}

// moveCursorUp moves the cursor to the line n lines above the current one.
func moveCursorUp(fd uintptr) func(io.Writer, uintptr, int) {
	// easy case, the terminal is cmd or psh, without redirection
	if isWindowsTerminal(fd) {
		return windowsMoveCursorUp
	}

	// assume we're running in mintty/cygwin
	return posixMoveCursorUp
}

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	procFillConsoleOutputCharacter = kernel32.NewProc("FillConsoleOutputCharacterW")
	procFillConsoleOutputAttribute = kernel32.NewProc("FillConsoleOutputAttribute")
)

// windowsClearCurrentLine removes all characters from the current line and
// resets the cursor position to the first column.
func windowsClearCurrentLine(_ io.Writer, fd uintptr) {
	var info windows.ConsoleScreenBufferInfo
	windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info)

	// clear the line
	cursor := windows.Coord{
		X: info.Window.Left,
		Y: info.CursorPosition.Y,
	}
	var count, w uint32
	count = uint32(info.Size.X)
	procFillConsoleOutputAttribute.Call(fd, uintptr(info.Attributes), uintptr(count), *(*uintptr)(unsafe.Pointer(&cursor)), uintptr(unsafe.Pointer(&w)))
	procFillConsoleOutputCharacter.Call(fd, uintptr(' '), uintptr(count), *(*uintptr)(unsafe.Pointer(&cursor)), uintptr(unsafe.Pointer(&w)))
}

// windowsMoveCursorUp moves the cursor to the line n lines above the current one.
func windowsMoveCursorUp(_ io.Writer, fd uintptr, n int) {
	var info windows.ConsoleScreenBufferInfo
	windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info)

	// move cursor up by n lines and to the first column
	windows.SetConsoleCursorPosition(windows.Handle(fd), windows.Coord{
		X: 0,
		Y: info.CursorPosition.Y - int16(n),
	})
}

// isWindowsTerminal return true if the file descriptor is a windows terminal (cmd, psh).
func isWindowsTerminal(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

func isPipe(fd uintptr) bool {
	typ, err := windows.GetFileType(windows.Handle(fd))
	return err == nil && typ == windows.FILE_TYPE_PIPE
}

func getFileNameByHandle(fd uintptr) (string, error) {
	type FILE_NAME_INFO struct {
		FileNameLength int32
		FileName       [windows.MAX_LONG_PATH]uint16
	}

	var fi FILE_NAME_INFO
	err := windows.GetFileInformationByHandleEx(windows.Handle(fd), windows.FileNameInfo, (*byte)(unsafe.Pointer(&fi)), uint32(unsafe.Sizeof(fi)))
	if err != nil {
		return "", err
	}

	filename := syscall.UTF16ToString(fi.FileName[:])
	return filename, nil
}

// CanUpdateStatus returns true if status lines can be printed, the process
// output is not redirected to a file or pipe.
func CanUpdateStatus(fd uintptr) bool {
	// easy case, the terminal is cmd or psh, without redirection
	if isWindowsTerminal(fd) {
		return true
	}

	// pipes require special handling
	if !isPipe(fd) {
		return false
	}

	fn, err := getFileNameByHandle(fd)
	if err != nil {
		return false
	}

	// inspired by https://github.com/RyanGlScott/mintty/blob/master/src/System/Console/MinTTY/Win32.hsc
	// terminal: \msys-dd50a72ab4668b33-pty0-to-master
	// pipe to cat: \msys-dd50a72ab4668b33-13244-pipe-0x16
	if (strings.HasPrefix(fn, "\\cygwin-") || strings.HasPrefix(fn, "\\msys-")) &&
		strings.Contains(fn, "-pty") && strings.HasSuffix(fn, "-master") {
		return true
	}

	return false
}
