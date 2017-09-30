package archiver_test

import (
	"context"
	"crypto/rand"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/mock"
	"github.com/restic/restic/internal/repository"
)

const parallelSaves = 50
const testSaveIndexTime = 100 * time.Millisecond
const testTimeout = 2 * time.Second

var DupID restic.ID

func randomID() restic.ID {
	if mrand.Float32() < 0.5 {
		return DupID
	}

	id := restic.ID{}
	_, err := io.ReadFull(rand.Reader, id[:])
	if err != nil {
		panic(err)
	}
	return id
}

// forgetfulBackend returns a backend that forgets everything.
func forgetfulBackend() restic.Backend {
	be := &mock.Backend{}

	be.TestFn = func(ctx context.Context, h restic.Handle) (bool, error) {
		return false, nil
	}

	be.LoadFn = func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
		return nil, errors.New("not found")
	}

	be.SaveFn = func(ctx context.Context, h restic.Handle, rd io.Reader) error {
		return nil
	}

	be.StatFn = func(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
		return restic.FileInfo{}, errors.New("not found")
	}

	be.RemoveFn = func(ctx context.Context, h restic.Handle) error {
		return nil
	}

	be.ListFn = func(ctx context.Context, t restic.FileType) <-chan string {
		ch := make(chan string)
		close(ch)
		return ch
	}

	be.DeleteFn = func(ctx context.Context) error {
		return nil
	}

	return be
}

func testArchiverDuplication(t *testing.T) {
	_, err := io.ReadFull(rand.Reader, DupID[:])
	if err != nil {
		t.Fatal(err)
	}

	repo := repository.New(forgetfulBackend())

	err = repo.Init(context.TODO(), "foo")
	if err != nil {
		t.Fatal(err)
	}

	arch := archiver.New(repo)

	wg := &sync.WaitGroup{}
	done := make(chan struct{})
	for i := 0; i < parallelSaves; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}

				id := randomID()

				if repo.Index().Has(id, restic.DataBlob) {
					continue
				}

				buf := make([]byte, 50)

				_, err := arch.Save(context.TODO(), restic.DataBlob, buf, id)
				if err != nil {
					t.Fatal(err)
				}
			}
		}()
	}

	saveIndex := func() {
		defer wg.Done()

		ticker := time.NewTicker(testSaveIndexTime)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				_, err := repo.SaveFullIndex(context.TODO())
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	wg.Add(1)
	go saveIndex()

	<-time.After(testTimeout)
	close(done)

	wg.Wait()

	_, err = repo.Flush()
	if err != nil {
		t.Fatal(err)
	}
}

func TestArchiverDuplication(t *testing.T) {
	for i := 0; i < 5; i++ {
		testArchiverDuplication(t)
	}
}
