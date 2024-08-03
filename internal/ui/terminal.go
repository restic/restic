package ui

// Terminal is used to write messages and display status lines which can be
// updated. See termstatus.Terminal for a concrete implementation.
type Terminal interface {
	Print(line string)
	Error(line string)
	SetStatus(lines []string)
	CanUpdateStatus() bool
}
