package main

import (
	"fmt"

	"github.com/restic/restic/internal/cache"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdWritable = &cobra.Command{
	Use:   "writable",
	Short: "Print whether the repository can be written to.",
	Long: `
The "writable" command is used to check whether the repository is writable or read-only".
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWritable(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdWritable)
}

func runWritable(gopts GlobalOptions, args []string) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}
	defer repo.Close()
	b := repo.Backend()
	switch x := b.(type) {
	case *cache.Backend:
		b = x.Backend
	default:
	}
	w, ok := b.(restic.Writabler)
	if !ok {
		return fmt.Errorf("The repository backend does not provide a definite answer. You just need to try.")
	}
	if w.Writable() {
		fmt.Println("The repository is considered to be writable.")
	} else {
		fmt.Println("The repository is read-only.")
	}
	return nil
}
