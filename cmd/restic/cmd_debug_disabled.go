//go:build !debug

package main

import "github.com/spf13/cobra"

func registerDebugCommand(_ *cobra.Command, _ *GlobalOptions) {
	// No commands to register in non-debug mode
}
