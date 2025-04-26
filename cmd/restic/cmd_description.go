package main

import (
	"context"

	"github.com/restic/restic/internal/ui/termstatus"
	"github.com/spf13/cobra"
)

func newDescriptionCommand() *cobra.Command {
	return nil
}

func runDescription(ctx context.Context, snapshot, description string, gopts GlobalOptions, term *termstatus.Terminal, args []string) error {
	return nil
}
