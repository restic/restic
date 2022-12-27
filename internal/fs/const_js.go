//go:build js
// +build js

package fs

// Flags to OpenFile wrapping those of the underlying system. Not all flags may
// be implemented on a given system.
const (
	O_RDONLY   int = 0
	O_WRONLY   int = 0
	O_RDWR     int = 0
	O_APPEND   int = 0
	O_CREATE   int = 0
	O_EXCL     int = 0
	O_SYNC     int = 0
	O_TRUNC    int = 0
	O_NONBLOCK int = 0
	O_NOFOLLOW int = 0
)
