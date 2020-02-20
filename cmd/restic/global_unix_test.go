// +build !windows

package main

import (
	"os"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestReadPasswordTerminalNotATTY(t *testing.T) {
	f, err := os.Open("/dev/zero")
	rtest.OK(t, err)
	defer f.Close()

	oldtty := devtty
	devtty = f.Name()
	defer func() { devtty = oldtty }()

	password, err := ReadPasswordTerminal("please enter password: ", false)
	rtest.Assert(t, err != nil,
		"ReadPasswordTerminal should refuse to read from a non-terminal")
	rtest.Equals(t, "", password)
}
