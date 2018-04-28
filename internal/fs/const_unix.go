// +build !windows

package fs

import "syscall"

// O_NOFOLLOW instructs the kernel to not follow symlinks when opening a file.
const O_NOFOLLOW int = syscall.O_NOFOLLOW
