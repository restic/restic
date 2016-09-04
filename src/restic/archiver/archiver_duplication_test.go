package archiver_test

import (
	"crypto/rand"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"restic/errors"

	"restic"
	"restic/archiver"
	"restic/mock"
	"restic/repository"
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

	be.TestFn = func(t restic.FileType, name string) (bool, error) {
		return false, nil
	}

	be.LoadFn = func(h restic.Handle, p []byte, off int64) (int, error) {
		return 0, errors.New("not found")
	}

	be.SaveFn = func(h restic.Handle, p []byte) error {
		return nil
	}

	be.StatFn = func(h restic.Handle) (restic.FileInfo, error) {
		return restic.FileInfo{}, errors.New("not found")
	}

	be.RemoveFn = func(t restic.FileType, name string) error {
		return nil
	}

	be.ListFn = func(t restic.FileType, done <-chan struct{}) <-chan string {
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

				err := arch.Save(restic.DataBlob, buf, id)
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
