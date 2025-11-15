//go:build !(darwin || freebsd || linux || windows)
// +build !darwin,!freebsd,!linux,!windows

package main

import (
	"github.com/restic/restic/internal/global"
	"github.com/spf13/cobra"
)

func registerMountCommand(_ *cobra.Command, _ *global.Options) {
	// Mount command not supported on these platforms
}
