//go:build !selfupdate

package main

import "github.com/spf13/cobra"

func registerSelfUpdateCommand(_ *cobra.Command, _ *GlobalOptions) {
	// No commands to register in non-selfupdate mode
}
