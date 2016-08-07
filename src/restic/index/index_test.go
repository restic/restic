package index

import (
	"restic"
	"restic/backend/local"
	"restic/repository"
	"testing"
	"time"
)

var (
	snapshotTime = time.Unix(1470492820, 207401672)
	snapshots    = 3
	depth        = 3
)

func createFilledRepo(t testing.TB, snapshots int) (*repository.Repository, func()) {
	repo, cleanup := repository.TestRepository(t)

	for i := 0; i < 3; i++ {
		restic.TestCreateSnapshot(t, repo, snapshotTime.Add(time.Duration(i)*time.Second), depth)
	}

	return repo, cleanup
}

func TestIndexNew(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3)
	defer cleanup()

	idx, err := New(repo)
	if err != nil {
		t.Fatalf("New() returned error %v", err)
	}

	if idx == nil {
		t.Fatalf("New() returned nil index")
	}
}

func TestIndexLoad(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3)
	defer cleanup()

	idx, err := Load(repo)
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}

	if idx == nil {
		t.Fatalf("Load() returned nil index")
	}
}

func openRepo(t testing.TB, dir, password string) *repository.Repository {
	b, err := local.Open(dir)
	if err != nil {
		t.Fatalf("open backend %v failed: %v", dir, err)
	}

	r := repository.New(b)
	err = r.SearchKey(password)
	if err != nil {
		t.Fatalf("unable to open repo with password: %v", err)
	}

	return r
}

func BenchmarkIndexNew(b *testing.B) {
	repo, cleanup := createFilledRepo(b, 3)
	defer cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx, err := New(repo)

		if err != nil {
			b.Fatalf("New() returned error %v", err)
		}

		if idx == nil {
			b.Fatalf("New() returned nil index")
		}
	}
}
