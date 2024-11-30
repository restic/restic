//go:build windows
// +build windows

package fs

// TODO honor flags when opening files

// O_NOFOLLOW is currently only interpreted by FS.OpenFile in metadataOnly mode and ignored by OpenFile.
// The value of the constant is invented and only for use within this fs package. It must not be used in other contexts.
// It must not conflict with the other O_* values from go/src/syscall/types_windows.go
const O_NOFOLLOW int = 0x40000000

// O_DIRECTORY is a noop on Windows.
const O_DIRECTORY int = 0
