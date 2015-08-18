package sftp

import (
	"syscall"
)

// ignoreSigIntProcAttr returns a default syscall.SysProcAttr
// on Windows.
func ignoreSigIntProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
