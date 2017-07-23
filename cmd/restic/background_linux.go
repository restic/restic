package main

import (
	"syscall"
	"unsafe"

	"restic/debug"
)

// IsProcessBackground returns true if it is running in the background or false if not
func IsProcessBackground() bool {
	var pid int
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdin), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&pid)))

	if err != 0 {
		debug.Log("Can't check if we are in the background. Using default behaviour. Error: %s\n", err.Error())
		return false
	}

	return pid != syscall.Getpgrp()
}
