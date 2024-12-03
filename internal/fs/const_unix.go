//go:build !windows
// +build !windows

package fs

import "syscall"

// O_NOFOLLOW instructs the kernel to not follow symlinks when opening a file.
const O_NOFOLLOW int = syscall.O_NOFOLLOW

// O_DIRECTORY instructs the kernel to only open directories.
const O_DIRECTORY int = syscall.O_DIRECTORY
