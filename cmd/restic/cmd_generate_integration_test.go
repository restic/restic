package main

import (
	"context"
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunGenerate(t testing.TB, gopts GlobalOptions, opts generateOptions) ([]byte, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		return runGenerate(opts, gopts, []string{}, gopts.term)
	})
	return buf.Bytes(), err
}

func TestGenerateStdout(t *testing.T) {
	testCases := []struct {
		name string
		opts generateOptions
	}{
		{"bash", generateOptions{BashCompletionFile: "-"}},
		{"fish", generateOptions{FishCompletionFile: "-"}},
		{"zsh", generateOptions{ZSHCompletionFile: "-"}},
		{"powershell", generateOptions{PowerShellCompletionFile: "-"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := testRunGenerate(t, GlobalOptions{}, tc.opts)
			rtest.OK(t, err)
			rtest.Assert(t, strings.Contains(string(output), "# "+tc.name+" completion for restic"), "has no expected completion header")
		})
	}

	t.Run("Generate shell completions to stdout for two shells", func(t *testing.T) {
		_, err := testRunGenerate(t, GlobalOptions{}, generateOptions{BashCompletionFile: "-", FishCompletionFile: "-"})
		rtest.Assert(t, err != nil, "generate shell completions to stdout for two shells fails")
	})
}
