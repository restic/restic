package fs

import "os"

// IsRegularFile returns true if fi belongs to a normal file. If fi is nil,
// false is returned.
func IsRegularFile(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}

	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}
