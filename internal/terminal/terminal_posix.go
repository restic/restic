package terminal

import (
	"bufio"
	"fmt"
	"os"
)

const (
	// PosixControlMoveCursorHome moves cursor to the first column
	PosixControlMoveCursorHome = "\r"
	// PosixControlMoveCursorUp moves cursor up one line
	PosixControlMoveCursorUp = "\x1b[1A"
	// PosixControlClearLine clears the current line
	PosixControlClearLine = "\x1b[2K"
)

// PosixClearCurrentLine removes all characters from the current line and resets the
// cursor position to the first column.
func PosixClearCurrentLine(w *bufio.Writer, _ uintptr) {
	_, err := w.WriteString(PosixControlMoveCursorHome + PosixControlClearLine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		return
	}
}

// PosixMoveCursorUp moves the cursor to the line n lines above the current one.
func PosixMoveCursorUp(w *bufio.Writer, _ uintptr, n int) {
	_, err := w.WriteString(PosixControlMoveCursorHome)
	for i := 0; i < n && err == nil; i++ {
		_, err = w.WriteString(PosixControlMoveCursorUp)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		return
	}
}
