// +build windows

package termstatus

import (
	"syscall"
	"unsafe"

	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sys/windows"
)

func (t *Terminal) clearCurrentLine() {
	switch t.termType {
	case termTypePosix:
		posixClearCurrentLine(t.wr)
	case termTypeWindows:
		windowsClearCurrentLine(uintptr(t.fd))
	}
}

func (t *Terminal) moveCursorUp(n int) {
	switch t.termType {
	case termTypePosix:
		posixMoveCursorUp(t.wr, n)
	case termTypeWindows:
		windowsMoveCursorUp(uintptr(t.fd), n)
	}
}

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	procSetConsoleCursorPosition   = kernel32.NewProc("SetConsoleCursorPosition")
	procFillConsoleOutputCharacter = kernel32.NewProc("FillConsoleOutputCharacterW")
	procFillConsoleOutputAttribute = kernel32.NewProc("FillConsoleOutputAttribute")
)

// windowsClearCurrentLine removes all characters from the current line and
// resets the cursor position to the first column.
func windowsClearCurrentLine(fd uintptr) {
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
func windowsMoveCursorUp(fd uintptr, n int) {
	var info windows.ConsoleScreenBufferInfo
	windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info)

	// move cursor up by n lines and to the first column
	info.CursorPosition.Y -= int16(n)
	info.CursorPosition.X = 0
	procSetConsoleCursorPosition.Call(fd, uintptr(*(*int32)(unsafe.Pointer(&info.CursorPosition))))
}

// initTermType sets t.termType and, if t is a terminal, t.fd.
func (t *Terminal) initTermType(fd int) {
	// easy case, the terminal is cmd or psh, without redirection
	if terminal.IsTerminal(fd) {
		t.fd = fd
		t.termType = termTypeWindows
		return
	}

	// Check if the output file type is a pipe.
	typ, err := windows.GetFileType(windows.Handle(fd))
	if err == nil && typ == windows.FILE_TYPE_PIPE {
		return
	}

	// Else, assume we're running in mintty/cygwin.
	t.fd = fd
	t.termType = termTypePosix
}
