// +build !windows

package sftp

import (
	"syscall"
)

func init() {
	// ignore signals sent to the parent (e.g. SIGINT)
	sysProcAttr = syscall.SysProcAttr{Setsid: true}
}
