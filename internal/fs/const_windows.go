//go:build windows
// +build windows

package fs

// TODO honor flags when opening files

// O_NOFOLLOW is a noop on Windows.
const O_NOFOLLOW int = 0

// O_DIRECTORY is a noop on Windows.
const O_DIRECTORY int = 0
