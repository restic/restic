// +build !windows

package main

import "os"

var devtty = "/dev/tty" // Variable so that the test can reset it.

// openTerminal opens the controlling terminal.
func openTerminal() (*controllingTerminal, error) {
	return os.OpenFile(devtty, os.O_RDWR, 0)
}

type controllingTerminal = os.File
