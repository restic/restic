//go:build !debug

package main

import "github.com/spf13/cobra"

func registerDebugCommand(_ *cobra.Command) {
	// No commands to register in non-debug mode
}
