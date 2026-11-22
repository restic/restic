package data_test

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/test"
)

func TestGroupByOptions(t *testing.T) {
	for _, exp := range []struct {
		from       string
		opts       data.SnapshotGroupByOptions
		normalized string
	}{
		{
			from:       "",
			opts:       data.SnapshotGroupByOptions{},
			normalized: "",
		},
		{
			from:       "host,paths",
			opts:       data.SnapshotGroupByOptions{Host: true, Path: true},
			normalized: "host,paths",
		},
		{
			from:       "host,path,tag",
			opts:       data.SnapshotGroupByOptions{Host: true, Path: true, Tag: true},
			normalized: "host,paths,tags",
		},
		{
			from:       "hosts,paths,tags",
			opts:       data.SnapshotGroupByOptions{Host: true, Path: true, Tag: true},
			normalized: "host,paths,tags",
		},
	} {
		var opts data.SnapshotGroupByOptions
		test.OK(t, opts.Set(exp.from))
		if !cmp.Equal(opts, exp.opts) {
			t.Errorf("unexpected opts %s", cmp.Diff(opts, exp.opts))
		}
		test.Equals(t, opts.String(), exp.normalized)
	}

	var opts data.SnapshotGroupByOptions
	err := opts.Set("tags,invalid")
	test.Assert(t, err != nil, "missing error on invalid tags")
	test.Assert(t, !opts.Host && !opts.Path && !opts.Tag, "unexpected opts %s %s %s", opts.Host, opts.Path, opts.Tag)
}

func TestSnapshotGroupKeyStringQuotes(t *testing.T) {
	ordinary := (&data.SnapshotGroupKey{Hostname: "host", Paths: []string{"/ok"}, Tags: []string{"tag"}}).String()
	test.Equals(t, "host host, path [/ok], tags [tag]", ordinary)

	hostile := (&data.SnapshotGroupKey{
		Hostname: "a\nb",
		Paths:    []string{"/ok", "p\nq"},
		Tags:     []string{"t\tu"},
	}).String()
	test.Equals(t, "host "+strconv.Quote("a\nb")+", path [/ok "+strconv.Quote("p\nq")+"], tags ["+strconv.Quote("t\tu")+"]", hostile)
}
