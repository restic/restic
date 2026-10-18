package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

func TestRestoreOverwriteCompletion(t *testing.T) {
	for _, flag := range []string{"--overwrite", "--overwrite="} {
		for _, tc := range []struct {
			prefix string
			want   string
		}{
			{"", "always\nif-changed\nif-newer\nnever\n:4\n"},
			{"if-", "if-changed\nif-newer\n:4\n"},
			{"never", "never\n:4\n"},
			{"unknown", ":4\n"},
		} {
			t.Run(flag+tc.prefix, func(t *testing.T) {
				cmd := newRootCommand(&global.Options{})
				var output bytes.Buffer
				cmd.SetOut(&output)
				cmd.SetErr(io.Discard)
				args := []string{"__complete", "restore", "latest", flag}
				if flag == "--overwrite" {
					args = append(args, tc.prefix)
				} else {
					args[len(args)-1] += tc.prefix
				}
				cmd.SetArgs(args)

				rtest.OK(t, cmd.Execute())
				rtest.Equals(t, tc.want, output.String())
			})
		}
	}
}
