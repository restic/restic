// +build !windows

package sftp

import (
	"syscall"
)

// ignoreSigIntProcAttr returns a syscall.SysProcAttr that
// disables SIGINT on parent.
func ignoreSigIntProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
