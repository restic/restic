// +build windows

package fs

// O_NOFOLLOW is a noop on Windows.
const O_NOFOLLOW int = 0
