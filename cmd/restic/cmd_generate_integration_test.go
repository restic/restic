package main

import (
	"bytes"
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

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
			buf := bytes.NewBuffer(nil)
			globalOptions.stdout = buf
			err := runGenerate(tc.opts, []string{})
			rtest.OK(t, err)
			completionString := buf.String()
			rtest.Assert(t, strings.Contains(completionString, "# "+tc.name+" completion for restic"), "has no expected completion header")
		})
	}

	t.Run("Generate shell completions to stdout for two shells", func(t *testing.T) {
		buf := bytes.NewBuffer(nil)
		globalOptions.stdout = buf
		opts := generateOptions{BashCompletionFile: "-", FishCompletionFile: "-"}
		err := runGenerate(opts, []string{})
		rtest.Assert(t, err != nil, "generate shell completions to stdout for two shells fails")
	})
}
