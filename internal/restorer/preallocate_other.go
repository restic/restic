// +build !linux,!darwin

package restorer

import "os"

func preallocateFile(wr *os.File, size int64) error {
	// Maybe truncate can help?
	// Windows: This calls SetEndOfFile which preallocates space on disk
	return wr.Truncate(size)
}
