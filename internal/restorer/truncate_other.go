//go:build !windows
// +build !windows

package restorer

import "os"

func truncateSparse(f *os.File, size int64) error {
	return f.Truncate(size)
}
