package repository

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/pack"
)

const parallelSaves = 20
const saveIndexTime = 100 * time.Millisecond
const testTimeout = 1 * time.Second

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

func testMasterIndex(t *testing.T) {
	_, err := io.ReadFull(rand.Reader, DupID[:])
	if err != nil {
		t.Fatal(err)
	}

	repo := New(forgetfulBackend())
	err = repo.Init("foo")
	if err != nil {
		t.Fatal(err)
	}

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

				if repo.Index().Has(id) {
					continue
				}

				buf := make([]byte, 50)

				err := repo.SaveFrom(pack.Data, &id, uint(len(buf)), bytes.NewReader(buf))
				if err != nil {
					t.Fatal(err)
				}
			}
		}()
	}

	saveIndexes := func() {
		defer wg.Done()

		ticker := time.NewTicker(saveIndexTime)
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
	go saveIndexes()

	<-time.After(testTimeout)
	close(done)

	wg.Wait()
}

func TestMasterIndex(t *testing.T) {
	for i := 0; i < 5; i++ {
		testMasterIndex(t)
	}
}
