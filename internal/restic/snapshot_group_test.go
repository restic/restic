package restic_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestGroupByOptions(t *testing.T) {
	for _, exp := range []struct {
		from       string
		opts       restic.SnapshotGroupByOptions
		normalized string
	}{
		{
			from:       "",
			opts:       restic.SnapshotGroupByOptions{},
			normalized: "",
		},
		{
			from:       "host,paths",
			opts:       restic.SnapshotGroupByOptions{Host: true, Path: true},
			normalized: "host,paths",
		},
		{
			from:       "host,path,tag",
			opts:       restic.SnapshotGroupByOptions{Host: true, Path: true, Tag: true},
			normalized: "host,paths,tags",
		},
		{
			from:       "hosts,paths,tags",
			opts:       restic.SnapshotGroupByOptions{Host: true, Path: true, Tag: true},
			normalized: "host,paths,tags",
		},
	} {
		var opts restic.SnapshotGroupByOptions
		test.OK(t, opts.Set(exp.from))
		if !cmp.Equal(opts, exp.opts) {
			t.Errorf("unexpeted opts %s", cmp.Diff(opts, exp.opts))
		}
		test.Equals(t, opts.String(), exp.normalized)
	}

	var opts restic.SnapshotGroupByOptions
	err := opts.Set("tags,invalid")
	test.Assert(t, err != nil, "missing error on invalid tags")
	test.Assert(t, !opts.Host && !opts.Path && !opts.Tag, "unexpected opts %s %s %s", opts.Host, opts.Path, opts.Tag)
}
