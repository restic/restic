// +build !linux

package fs

import "os"

// Open opens a file for reading.
func Open(name string) (File, error) {
	f, err := os.OpenFile(name, os.O_RDONLY, 0)
	return osFile{File: f}, err
}

// osFile wraps an *os.File and adds a no-op ClearCache() method.
type osFile struct {
	*os.File
}

func (osFile) ClearCache() error {
	return nil
}
