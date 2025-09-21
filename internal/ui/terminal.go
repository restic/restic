package ui

import (
	"context"
	"io"
)

// Terminal is used to write messages and display status lines which can be
// updated. See termstatus.Terminal for a concrete implementation.
type Terminal interface {
	// Print writes a line to the terminal. Appends a newline if not present.
	Print(line string)
	// Error writes an error to the terminal. Appends a newline if not present.
	Error(line string)
	// SetStatus sets the status lines to the terminal.
	SetStatus(lines []string)
	// CanUpdateStatus returns true if the terminal can update the status lines.
	CanUpdateStatus() bool

	// InputRaw returns the input reader.
	InputRaw() io.ReadCloser
	// InputIsTerminal returns true if the input is a terminal.
	InputIsTerminal() bool
	// ReadPassword reads the password from the terminal.
	ReadPassword(ctx context.Context, prompt string) (string, error)

	// OutputWriter returns a output writer that is safe for concurrent use with
	// other output methods. Output is only shown after a line break.
	OutputWriter() io.Writer
	// OutputRaw returns the raw output writer. Should only be used if there is no
	// other option. Must not be used in combination with Print, Error, SetStatus
	// or any other method that writes to the terminal.
	OutputRaw() io.Writer
	// OutputIsTerminal returns true if the output is a terminal.
	OutputIsTerminal() bool
}
