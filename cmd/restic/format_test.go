package main

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestFormatNode(t *testing.T) {
	// overwrite time zone to ensure the data is formatted reproducibly
	tz := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = tz
	}()

	testPath := "/test/path"
	node := restic.Node{
		Name:    "baz",
		Type:    "file",
		Size:    14680064,
		UID:     1000,
		GID:     2000,
		ModTime: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	for _, c := range []struct {
		path string
		restic.Node
		long   bool
		human  bool
		expect string
	}{
		{
			path:   testPath,
			Node:   node,
			long:   false,
			human:  false,
			expect: testPath,
		},
		{
			path:   testPath,
			Node:   node,
			long:   true,
			human:  false,
			expect: "----------  1000  2000 14680064 2020-01-02 03:04:05 " + testPath,
		},
		{
			path:   testPath,
			Node:   node,
			long:   true,
			human:  true,
			expect: "----------  1000  2000 14.000 MiB 2020-01-02 03:04:05 " + testPath,
		},
	} {
		r := formatNode(c.path, &c.Node, c.long, c.human)
		rtest.Equals(t, c.expect, r)
	}
}
