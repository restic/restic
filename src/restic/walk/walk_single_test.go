package walk_test

import (
	"context"
	"path"
	"path/filepath"
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
		id    restic.ID
	}{
		{"", 1, restic.TestParseID("937a2f64f736c64ee700c6ab06f840c68c94799c288146a0e81e07f4c94254da")},
		{"testdata", 1, restic.TestParseID("95a8e976b3b3d3b5f9e5479ec931bb3b0ae03c8f9804534743c8eebd9e955cfd")},
		{"testdata/0", 1, restic.TestParseID("69b0e8c74f3e7c9767c99ffd1f7f2030191bab3a62ab3094d38d99d9c7b1bdf5")},
		{"testdata/0/0", 10, restic.TestParseID("d9ec2f62d835bddd351c05bf3d7cbccccacf7d291a6c214ce25afeefd097db19")},
		{"testdata/0/0/0", 128, restic.TestParseID("993224dfff304496104a794dde6cdd71ba6b32db033d46c8fda6cf9468db3b39")},
		{"testdata/0/0/1", 128, restic.TestParseID("7875683370ea7651a0b5ccc71def952f1afb44d3d952f4bb29a5638763ec9418")},
		{"testdata/0/0/2", 128, restic.TestParseID("a9be22bf72fdd6f9f4d96ad3c935c1c714922418b217318824913b45673e7584")},
		{"testdata/0/0/3", 128, restic.TestParseID("bb07f258499694e5829ad65fd50187be0898207ed4b70437f273ae4427898355")},
		{"testdata/0/0/4", 128, restic.TestParseID("582deeab0d776f73696ae1d2c583328eb1da43cf70bbf4cbab9abeb8379e3de4")},
		{"testdata/0/0/5", 128, restic.TestParseID("9ec920f50c47448d22e3bf52382086be67e63f36f8e62adc2a0c1140236db49d")},
		{"testdata/0/0/6", 128, restic.TestParseID("3177e24f6a14ed28ee2490276309c7518cc8b5cd884d9a51abae28a5aea357df")},
		{"testdata/0/0/7", 128, restic.TestParseID("a86b1abbf1bc8941be24b91379620b302814af6555e84a5fe73a9a14647318d2")},
		{"testdata/0/0/8", 128, restic.TestParseID("a18dd6362672ba7417db2d90fd1441504ea739fca7e8bbe0d4a15008c64264c2")},
		{"testdata/0/0/9", 69, restic.TestParseID("a068b2ca9d2e1c4127bdead3194920561311be432b0b7f5c6547754cc1ec300a")},
	}

	repodir, cleanup := test.Env(t, repoFixture)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)
	if err := repo.LoadIndex(context.TODO()); err != nil {
		t.Fatal(err)
	}

	root := restic.TestParseID("937a2f64f736c64ee700c6ab06f840c68c94799c288146a0e81e07f4c94254da")

	i := 0
	err := walk.Walk(context.TODO(), repo, root, func(dir string, id restic.ID, tree *restic.Tree, err error) error {
		if err != nil {
			return err
		}

		test := testItems[i]
		want := filepath.Join(path.Split(test.dir))

		if dir != want {
			t.Errorf("path %d is wrong: want %q, got %q", i, want, dir)
		}

		if !test.id.Equal(id) {
			t.Errorf("path %d, ID is wrong: want %s, got %s", i, test.id, id)
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
		err := walk.Walk(context.TODO(), dr, root, func(path string, id restic.ID, tree *restic.Tree, err error) error {
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
