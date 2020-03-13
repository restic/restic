// +build windows

package termstatus

import (
	"io"
	"syscall"
	"unsafe"

	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sys/windows"
)

// clearCurrentLine removes all characters from the current line and resets the
// cursor position to the first column.
func clearCurrentLine(wr io.Writer, fd uintptr) func(io.Writer, uintptr) {
	// easy case, the terminal is cmd or psh, without redirection
	if isWindowsTerminal(fd) {
		return windowsClearCurrentLine
	}

	// assume we're running in mintty/cygwin
	return posixClearCurrentLine
}

// moveCursorUp moves the cursor to the line n lines above the current one.
func moveCursorUp(wr io.Writer, fd uintptr) func(io.Writer, uintptr, int) {
	// easy case, the terminal is cmd or psh, without redirection
	if isWindowsTerminal(fd) {
		return windowsMoveCursorUp
	}

	// assume we're running in mintty/cygwin
	return posixMoveCursorUp
}

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	procSetConsoleCursorPosition   = kernel32.NewProc("SetConsoleCursorPosition")
	procFillConsoleOutputCharacter = kernel32.NewProc("FillConsoleOutputCharacterW")
	procFillConsoleOutputAttribute = kernel32.NewProc("FillConsoleOutputAttribute")
)

// windowsClearCurrentLine removes all characters from the current line and
// resets the cursor position to the first column.
func windowsClearCurrentLine(wr io.Writer, fd uintptr) {
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
func windowsMoveCursorUp(wr io.Writer, fd uintptr, n int) {
	var info windows.ConsoleScreenBufferInfo
	windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info)

	// move cursor up by n lines and to the first column
	info.CursorPosition.Y -= int16(n)
	info.CursorPosition.X = 0
	procSetConsoleCursorPosition.Call(fd, uintptr(*(*int32)(unsafe.Pointer(&info.CursorPosition))))
}

// isWindowsTerminal return true if the file descriptor is a windows terminal (cmd, psh).
func isWindowsTerminal(fd uintptr) bool {
	return terminal.IsTerminal(int(fd))
}

func isPipe(fd uintptr) bool {
	typ, err := windows.GetFileType(windows.Handle(fd))
	return err == nil && typ == windows.FILE_TYPE_PIPE
}

// canUpdateStatus returns true if status lines can be printed, the process
// output is not redirected to a file or pipe.
func canUpdateStatus(fd uintptr) bool {
	// easy case, the terminal is cmd or psh, without redirection
	if isWindowsTerminal(fd) {
		return true
	}

	// check if the output file type is a pipe (0x0003)
	if isPipe(fd) {
		return false
	}

	// assume we're running in mintty/cygwin
	return true
}
