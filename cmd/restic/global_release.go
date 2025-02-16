//go:build !debug && !profile
// +build !debug,!profile

package main

import "github.com/spf13/cobra"

func registerProfiling(_ *cobra.Command) {
	// No profiling in release mode
}
