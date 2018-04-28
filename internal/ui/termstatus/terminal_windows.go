// +build windows

package termstatus

import (
	"io"
	"syscall"
	"unsafe"
)

// clearLines clears the current line and n lines above it.
func clearLines(wr io.Writer, fd uintptr) clearLinesFunc {
	// easy case, the terminal is cmd or psh, without redirection
	if isWindowsTerminal(fd) {
		return windowsClearLines
	}

	// check if the output file type is a pipe (0x0003)
	if getFileType(fd) != fileTypePipe {
		// return empty func, update state is not possible on this terminal
		return func(io.Writer, uintptr, int) {}
	}

	// assume we're running in mintty/cygwin
	return posixClearLines
}

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procSetConsoleCursorPosition   = kernel32.NewProc("SetConsoleCursorPosition")
	procFillConsoleOutputCharacter = kernel32.NewProc("FillConsoleOutputCharacterW")
	procFillConsoleOutputAttribute = kernel32.NewProc("FillConsoleOutputAttribute")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procGetFileType                = kernel32.NewProc("GetFileType")
)

type (
	short int16
	word  uint16
	dword uint32

	coord struct {
		x short
		y short
	}
	smallRect struct {
		left   short
		top    short
		right  short
		bottom short
	}
	consoleScreenBufferInfo struct {
		size              coord
		cursorPosition    coord
		attributes        word
		window            smallRect
		maximumWindowSize coord
	}
)

// windowsClearLines clears the current line and n lines above it.
func windowsClearLines(wr io.Writer, fd uintptr, n int) {
	var info consoleScreenBufferInfo
	procGetConsoleScreenBufferInfo.Call(fd, uintptr(unsafe.Pointer(&info)))

	for i := 0; i <= n; i++ {
		// clear the line
		cursor := coord{
			x: info.window.left,
			y: info.cursorPosition.y - short(i),
		}
		var count, w dword
		count = dword(info.size.x)
		procFillConsoleOutputAttribute.Call(fd, uintptr(info.attributes), uintptr(count), *(*uintptr)(unsafe.Pointer(&cursor)), uintptr(unsafe.Pointer(&w)))
		procFillConsoleOutputCharacter.Call(fd, uintptr(' '), uintptr(count), *(*uintptr)(unsafe.Pointer(&cursor)), uintptr(unsafe.Pointer(&w)))
	}

	// move cursor up by n lines and to the first column
	info.cursorPosition.y -= short(n)
	info.cursorPosition.x = 0
	procSetConsoleCursorPosition.Call(fd, uintptr(*(*int32)(unsafe.Pointer(&info.cursorPosition))))
}

// getTermSize returns the dimensions of the given terminal.
// the code is taken from "golang.org/x/crypto/ssh/terminal"
func getTermSize(fd uintptr) (width, height int, err error) {
	var info consoleScreenBufferInfo
	_, _, e := syscall.Syscall(procGetConsoleScreenBufferInfo.Addr(), 2, fd, uintptr(unsafe.Pointer(&info)), 0)
	if e != 0 {
		return 0, 0, error(e)
	}
	return int(info.size.x), int(info.size.y), nil
}

// isWindowsTerminal return true if the file descriptor is a windows terminal (cmd, psh).
func isWindowsTerminal(fd uintptr) bool {
	var st uint32
	r, _, e := syscall.Syscall(procGetConsoleMode.Addr(), 2, fd, uintptr(unsafe.Pointer(&st)), 0)
	return r != 0 && e == 0
}

const fileTypePipe = 0x0003

// getFileType returns the file type for the given fd.
// https://msdn.microsoft.com/de-de/library/windows/desktop/aa364960(v=vs.85).aspx
func getFileType(fd uintptr) int {
	r, _, e := syscall.Syscall(procGetFileType.Addr(), 1, fd, 0, 0)
	if e != 0 {
		return 0
	}
	return int(r)
}

// canUpdateStatus returns true if status lines can be printed, the process
// output is not redirected to a file or pipe.
func canUpdateStatus(fd uintptr) bool {
	// easy case, the terminal is cmd or psh, without redirection
	if isWindowsTerminal(fd) {
		return true
	}

	// check if the output file type is a pipe (0x0003)
	if getFileType(fd) != fileTypePipe {
		return false
	}

	// assume we're running in mintty/cygwin
	return true
}
