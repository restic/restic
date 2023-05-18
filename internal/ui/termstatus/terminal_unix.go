//go:build !windows
// +build !windows

package termstatus

import (
	"io"
	"os"

	"golang.org/x/term"
)

// clearCurrentLine removes all characters from the current line and resets the
// cursor position to the first column.
func clearCurrentLine(_ uintptr) func(io.Writer, uintptr) {
	return posixClearCurrentLine
}

// moveCursorUp moves the cursor to the line n lines above the current one.
func moveCursorUp(_ uintptr) func(io.Writer, uintptr, int) {
	return posixMoveCursorUp
}

// CanUpdateStatus returns true if status lines can be printed, the process
// output is not redirected to a file or pipe.
func CanUpdateStatus(fd uintptr) bool {
	if !term.IsTerminal(int(fd)) {
		return false
	}
	term := os.Getenv("TERM")
	if term == "" {
		return false
	}
	// TODO actually read termcap db and detect if terminal supports what we need
	return term != "dumb"
}
