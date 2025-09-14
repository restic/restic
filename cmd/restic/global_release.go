//go:build !debug && !profile
// +build !debug,!profile

package main

import (
	"io"

	"github.com/spf13/cobra"
)

func registerProfiling(_ *cobra.Command, _ io.Writer) {
	// No profiling in release mode
}
