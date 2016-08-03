package restic_test

import (
	"crypto/rand"
	"errors"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"restic"
	"restic/backend"
	"restic/pack"
	"restic/repository"
)

const parallelSaves = 50
const testSaveIndexTime = 100 * time.Millisecond
const testTimeout = 2 * time.Second

var DupID backend.ID

func randomID() backend.ID {
	if mrand.Float32() < 0.5 {
		return DupID
	}

	id := backend.ID{}
	_, err := io.ReadFull(rand.Reader, id[:])
	if err != nil {
		panic(err)
	}
	return id
}

// forgetfulBackend returns a backend that forgets everything.
func forgetfulBackend() backend.Backend {
	be := &backend.MockBackend{}

	be.TestFn = func(t backend.Type, name string) (bool, error) {
		return false, nil
	}

	be.LoadFn = func(h backend.Handle, p []byte, off int64) (int, error) {
		return 0, errors.New("not found")
	}

	be.SaveFn = func(h backend.Handle, p []byte) error {
		return nil
	}

	be.StatFn = func(h backend.Handle) (backend.BlobInfo, error) {
		return backend.BlobInfo{}, errors.New("not found")
	}

	be.RemoveFn = func(t backend.Type, name string) error {
		return nil
	}

	be.ListFn = func(t backend.Type, done <-chan struct{}) <-chan string {
		ch := make(chan string)
		close(ch)
		return ch
	}

	be.DeleteFn = func() error {
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

	err = repo.Init("foo")
	if err != nil {
		t.Fatal(err)
	}

	arch := restic.NewArchiver(repo)

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

				if repo.Index().Has(id, pack.Data) {
					continue
				}

				buf := make([]byte, 50)

				err := arch.Save(pack.Data, buf, id)
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
				err := repo.SaveFullIndex()
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
}

func TestArchiverDuplication(t *testing.T) {
	for i := 0; i < 5; i++ {
		testArchiverDuplication(t)
	}
}
