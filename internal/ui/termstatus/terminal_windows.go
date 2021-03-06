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
	return windowsClearCurrentLine
}

// moveCursorUp moves the cursor to the line n lines above the current one.
func moveCursorUp(wr io.Writer, fd uintptr) func(io.Writer, uintptr, int) {
	return windowsMoveCursorUp
}

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
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
	windows.SetConsoleCursorPosition(windows.Handle(fd), windows.Coord{
		X: 0,
		Y: info.CursorPosition.Y - int16(n),
	})
}

// isWindowsTerminal return true if the file descriptor is a windows terminal (cmd, psh, mintty).
func isWindowsTerminal(fd uintptr) bool {
	// IsTerminal does not return true if the output is piped to a file or other command
	return terminal.IsTerminal(int(fd))
}

// canUpdateStatus returns true if status lines can be printed, the process
// output is not redirected to a file or pipe.
func canUpdateStatus(fd uintptr) bool {
	// if running in a terminal, allow status updates
	return isWindowsTerminal(fd)
}
