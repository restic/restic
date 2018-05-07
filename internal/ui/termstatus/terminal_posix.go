package termstatus

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

const (
	posixControlMoveCursorHome = "\r"
	posixControlMoveCursorUp   = "\x1b[1A"
	posixControlClearLine      = "\x1b[2K"
)

// posixClearCurrentLine removes all characters from the current line and resets the
// cursor position to the first column.
func posixClearCurrentLine(wr io.Writer, fd uintptr) {
	// clear current line
	_, err := wr.Write([]byte(posixControlMoveCursorHome + posixControlClearLine))
	if err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		return
	}
}

// posixMoveCursorUp moves the cursor to the line n lines above the current one.
func posixMoveCursorUp(wr io.Writer, fd uintptr, n int) {
	data := []byte(posixControlMoveCursorHome)
	data = append(data, bytes.Repeat([]byte(posixControlMoveCursorUp), n)...)
	_, err := wr.Write(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		return
	}
}
