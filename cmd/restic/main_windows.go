// +build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"
)

func init() {
	f := cmdRoot.PersistentPreRunE
	cmdRoot.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		if h, err := PowerCreateRequest(fmt.Sprintf("Restic %s in progress.", c.Name())); err == nil {
			_ = PowerSetRequest(h, PowerRequestSystemRequired)
			_ = PowerSetRequest(h, PowerRequestExecutionRequired)
		}

		return f(c, args)
	}
}

type PowerRequestType int

//nolint:deadcode
const (
	PowerRequestDisplayRequired PowerRequestType = iota
	PowerRequestSystemRequired
	PowerRequestAwayModeRequired
	PowerRequestExecutionRequired
)

func PowerCreateRequest(reason string) (handle windows.Handle, err error) {
	if err := procPowerCreateRequest.Find(); err != nil {
		return windows.InvalidHandle, err
	}
	var reasonPtr *uint16
	if reasonPtr, err = windows.UTF16PtrFromString(reason); err != nil {
		return windows.InvalidHandle, err
	}
	ctx := powerReasonContext{
		Version:            0, // POWER_REQUEST_CONTEXT_VERSION
		Flags:              1, // POWER_REQUEST_CONTEXT_SIMPLE_STRING
		SimpleReasonString: reasonPtr,
	}
	r0, _, e1 := syscall.Syscall(procPowerCreateRequest.Addr(), 1, uintptr(unsafe.Pointer(&ctx)), 0, 0)
	handle = windows.Handle(r0)
	if handle == windows.InvalidHandle {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}

	return
}

func PowerSetRequest(request windows.Handle, requestType PowerRequestType) (err error) {
	r1, _, e1 := syscall.Syscall(procPowerSetRequest.Addr(), 2, uintptr(request), uintptr(requestType), 0)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}

	return
}

type powerReasonContext struct {
	Version            uint32
	Flags              uint32
	SimpleReasonString *uint16
	_                  uint32
	_                  uint32
	_                  **uint16
}

var (
	kernel32Dll            = windows.NewLazySystemDLL("kernel32.dll")
	procPowerCreateRequest = kernel32Dll.NewProc("PowerCreateRequest")
	procPowerSetRequest    = kernel32Dll.NewProc("PowerSetRequest")
)
