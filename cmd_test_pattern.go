package main

import (
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newTestPatternCommand(globalOptions *global.Options) *cobra.Command {
	var opts TestPatternOptions

	cmd := &cobra.Command{
		Use:   "test-pattern [flags] PATTERN PATH",
		Short: "Test a pattern against a path",
		Long: `
The "test-pattern" command tests whether a given pattern matches a specific
path. It uses the same pattern matching logic as the "find", "backup", and
"restore" commands, supporting the '**' recursive wildcard in addition to
filepath.Match patterns.

This is useful for verifying exclude/include patterns before running a backup.

EXIT STATUS
===========

Exit status is 0 if the pattern matched the path.
Exit status is 1 if the pattern did not match.
`,
		Example: `restic test-pattern '*.go' /home/user/main.go
restic test-pattern --ignore-case '*.GO' /home/user/main.go
restic test-pattern '**/.git/**' /home/user/project/.git/config
restic test-pattern '/home/user/*.txt' /home/user/readme.txt`,
		DisableAutoGenTag: true,
		GroupID:           cmdGroupDefault,
		RunE: func(_ *cobra.Command, args []string) error {
			return runTestPattern(opts, *globalOptions, args)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// TestPatternOptions collects all options for the test-pattern command.
type TestPatternOptions struct {
	CaseInsensitive bool
}

func (opts *TestPatternOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVarP(&opts.CaseInsensitive, "ignore-case", "i", false, "ignore case for pattern")
}

func runTestPattern(opts TestPatternOptions, gopts global.Options, args []string) error {
	if len(args) != 2 {
		return errors.Fatal("wrong number of arguments, expecting: test-pattern [flags] PATTERN PATH")
	}

	pattern := args[0]
	path := args[1]

	normalizedPattern := pattern
	normalizedPath := path
	if opts.CaseInsensitive {
		normalizedPattern = strings.ToLower(pattern)
		normalizedPath = strings.ToLower(path)
	}

	matched, err := filter.Match(normalizedPattern, normalizedPath)
	if err != nil {
		return errors.Fatalf("error matching pattern: %v", err)
	}

	childMayMatch, err := filter.ChildMatch(normalizedPattern, normalizedPath)
	if err != nil {
		return errors.Fatalf("error testing child match: %v", err)
	}

	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)

	if opts.CaseInsensitive {
		printer.S("pattern    : %s (case-insensitive)\n", pattern)
	} else {
		printer.S("pattern    : %s\n", pattern)
	}
	printer.S("path       : %s\n", path)
	printer.S("match      : %v\n", matched)
	printer.S("child match: %v\n", childMayMatch)

	if !matched {
		return errors.Fatal("pattern did not match")
	}

	return nil
}
