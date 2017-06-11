package walk_test

import (
	"context"
	"restic"
	"restic/repository"
	"restic/test"
	"restic/walk"
	"testing"
	"time"
)

func TestWalk(t *testing.T) {
	var testItems = []struct {
		dir   string
		nodes int
	}{
		{"", 1},
		{"testdata", 1},
		{"testdata/0", 1},
		{"testdata/0/0", 10},
		{"testdata/0/0/0", 128},
		{"testdata/0/0/1", 128},
		{"testdata/0/0/2", 128},
		{"testdata/0/0/3", 128},
		{"testdata/0/0/4", 128},
		{"testdata/0/0/5", 128},
		{"testdata/0/0/6", 128},
		{"testdata/0/0/7", 128},
		{"testdata/0/0/8", 128},
		{"testdata/0/0/9", 69},
	}

	repodir, cleanup := test.Env(t, repoFixture)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)
	if err := repo.LoadIndex(context.TODO()); err != nil {
		t.Fatal(err)
	}

	root := restic.TestParseID("937a2f64f736c64ee700c6ab06f840c68c94799c288146a0e81e07f4c94254da")

	i := 0
	err := walk.Walk(context.TODO(), repo, root, func(dir string, tree *restic.Tree, err error) error {
		if err != nil {
			return err
		}

		test := testItems[i]
		if dir != test.dir {
			t.Errorf("path %d is wrong: want %q, got %q", i, test.dir, dir)
		}

		if len(tree.Nodes) != test.nodes {
			t.Errorf("number of nodes for %d is wrong: want %d, got %d", i, test.nodes, len(tree.Nodes))
		}

		i++

		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
}

func BenchmarkDelayedWalk(t *testing.B) {
	repodir, cleanup := test.Env(t, repoFixture)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)
	if err := repo.LoadIndex(context.TODO()); err != nil {
		t.Fatal(err)
	}

	root := restic.TestParseID("937a2f64f736c64ee700c6ab06f840c68c94799c288146a0e81e07f4c94254da")

	dr := delayRepo{repo, 10 * time.Millisecond}

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		err := walk.Walk(context.TODO(), dr, root, func(path string, tree *restic.Tree, err error) error {
			if err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			t.Fatal(err)
		}
	}
}
