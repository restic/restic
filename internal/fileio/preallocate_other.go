//go:build !linux && !darwin

package fileio

import "os"

// preallocateFile is the platform's own call (windows: SetEndOfFile via truncate);
// PreallocateFile in preallocate.go wraps it with the deadline every platform needs.
func preallocateFile(wr *os.File, size int64) error {
	// Maybe truncate can help?
	// Windows: This calls SetEndOfFile which preallocates space on disk
	return wr.Truncate(size)
}
