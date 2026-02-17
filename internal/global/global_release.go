//go:build !debug && !profile

package global

import (
	"io"

	"github.com/spf13/cobra"
)

func RegisterProfiling(_ *cobra.Command, _ io.Writer) {
	// No profiling in release mode
}
