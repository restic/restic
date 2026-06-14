//go:build !linux && !darwin

package fileio

import "os"

func PreallocateFile(wr *os.File, size int64) error {
	// Maybe truncate can help?
	// Windows: This calls SetEndOfFile which preallocates space on disk
	return wr.Truncate(size)
}
