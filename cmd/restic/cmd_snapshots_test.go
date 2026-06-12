package main

import (
	"strings"
	"testing"

	"github.com/restic/restic/internal/data"
	rtest "github.com/restic/restic/internal/test"
	"github.com/spf13/pflag"
)

func TestSnapshotOptionsFinalize(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want data.SnapshotGroupByOptions
	}{
		{
			name: "default group by when latest set",
			args: []string{"--latest", "1"},
			want: data.SnapshotGroupByOptions{Host: true, Path: true},
		},
		{
			name: "preserve explicit group by",
			args: []string{"--latest", "1", "--group-by", "host"},
			want: data.SnapshotGroupByOptions{Host: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			set := pflag.NewFlagSet("test", pflag.PanicOnError)
			opts := &SnapshotOptions{}
			opts.AddFlags(set)
			rtest.OK(t, set.Parse(test.args))
			rtest.OK(t, opts.Finalize())
			rtest.Equals(t, test.want, opts.GroupBy)
		})
	}
}

// Regression test for #2979: no snapshots should print as [], not null.
func TestEmptySnapshotGroupJSON(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		var w strings.Builder
		err := printSnapshotGroupJSON(&w, nil, grouped)
		rtest.OK(t, err)

		rtest.Equals(t, "[]", strings.TrimSpace(w.String()))
	}
}
