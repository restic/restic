// +build !windows

package termstatus

import (
	"io"
	"os"

	"golang.org/x/sys/unix"

	isatty "github.com/mattn/go-isatty"
)

// clearCurrentLine removes all characters from the current line and resets the
// cursor position to the first column.
func clearCurrentLine(wr io.Writer, fd uintptr) func(io.Writer, uintptr) {
	return posixClearCurrentLine
}

// moveCursorUp moves the cursor to the line n lines above the current one.
func moveCursorUp(wr io.Writer, fd uintptr) func(io.Writer, uintptr, int) {
	return posixMoveCursorUp
}

// canUpdateStatus returns true if status lines can be printed, the process
// output is not redirected to a file or pipe.
func canUpdateStatus(fd uintptr) bool {
	if !isatty.IsTerminal(fd) {
		return false
	}
	term := os.Getenv("TERM")
	if term == "" {
		return false
	}
	// TODO actually read termcap db and detect if terminal supports what we need
	return term != "dumb"
}

// getTermSize returns the dimensions of the given terminal.
// the code is taken from "golang.org/x/crypto/ssh/terminal"
func getTermSize(fd uintptr) (width, height int, err error) {
	ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	if err != nil {
		return -1, -1, err
	}
	return int(ws.Col), int(ws.Row), nil
}
