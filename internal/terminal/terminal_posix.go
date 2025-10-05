package terminal

import (
	"bytes"
	"fmt"
	"io"
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
func PosixClearCurrentLine(wr io.Writer, _ uintptr) error {
	// clear current line
	_, err := wr.Write([]byte(PosixControlMoveCursorHome + PosixControlClearLine))
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

// PosixMoveCursorUp moves the cursor to the line n lines above the current one.
func PosixMoveCursorUp(wr io.Writer, _ uintptr, n int) error {
	data := []byte(PosixControlMoveCursorHome)
	data = append(data, bytes.Repeat([]byte(PosixControlMoveCursorUp), n)...)
	_, err := wr.Write(data)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}
