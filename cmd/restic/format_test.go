package main

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestFormatNode(t *testing.T) {
	for _, c := range []struct {
		path string
		restic.Node
		long   bool
		human  bool
		expect string
	}{
		{
			path: "/test/path",
			Node: restic.Node{
				Name:    "baz",
				Type:    "file",
				Size:    14680064,
				UID:     1000,
				GID:     2000,
				ModTime: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
			},
			long:   false,
			human:  false,
			expect: "/test/path",
		},
		{
			path: "/test/path",
			Node: restic.Node{
				Name:    "baz",
				Type:    "file",
				Size:    14680064,
				UID:     1000,
				GID:     2000,
				ModTime: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
			},
			long:   true,
			human:  false,
			expect: "----------  1000  2000 14680064 2020-01-01 22:04:05 /test/path",
		},
		{
			path: "/test/path",
			Node: restic.Node{
				Name:    "baz",
				Type:    "file",
				Size:    14680064,
				UID:     1000,
				GID:     2000,
				ModTime: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
			},
			long:   true,
			human:  true,
			expect: "----------  1000  2000 14.000 MiB 2020-01-01 22:04:05 /test/path",
		},
	} {
		r := formatNode(c.path, &c.Node, c.long, c.human)
		rtest.Equals(t, r, c.expect)
	}
}
