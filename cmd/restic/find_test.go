package main

import (
	"testing"

	"github.com/restic/restic/internal/data"
	rtest "github.com/restic/restic/internal/test"
	"github.com/spf13/pflag"
)

func TestSnapshotFilter(t *testing.T) {
	for _, test := range []struct {
		name     string
		args     []string
		expected []string
		env      string
	}{
		{
			"no value",
			[]string{},
			nil,
			"",
		},
		{
			"args only",
			[]string{"--host", "abc"},
			[]string{"abc"},
			"",
		},
		{
			"env default",
			[]string{},
			[]string{"def"},
			"def",
		},
		{
			"both",
			[]string{"--host", "abc"},
			[]string{"abc"},
			"def",
		},
		{
			"env set, empty flag overrides",
			[]string{"--host", ""},
			nil, // empty host filter means all hosts
			"envhost",
		},
		{
			"env set, multiple flags override",
			[]string{"--host", "host1", "--host", "host2"},
			[]string{"host1", "host2"},
			"envhost",
		},
		{
			"env set, multiple hosts including empty",
			[]string{"--host", "host1", "--host", ""},
			[]string{"host1", ""},
			"envhost",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("RESTIC_HOST", test.env)

			for _, mode := range []bool{false, true} {
				set := pflag.NewFlagSet("test", pflag.PanicOnError)
				flt := &data.SnapshotFilter{}
				if mode {
					initMultiSnapshotFilter(set, flt, false)
				} else {
					initSingleSnapshotFilter(set, flt)
				}
				err := set.Parse(test.args)
				rtest.OK(t, err)

				// Apply the finalization logic to handle env defaults
				finalizeSnapshotFilter(flt)

				rtest.Equals(t, test.expected, flt.Hosts, "unexpected hosts")
			}
		})
	}
}
