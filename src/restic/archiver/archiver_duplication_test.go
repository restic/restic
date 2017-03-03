package archiver_test

import (
	"crypto/rand"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"restic/backend/mem"
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

// forgetfulBackend returns a backend that forgets everything except keys and
// config.
func forgetfulBackend() restic.Backend {
	be := mem.New()

	mock := &mock.Backend{}

	mock.TestFn = func(h restic.Handle) (bool, error) {
		if h.Type == restic.ConfigFile || h.Type == restic.KeyFile {
			return be.Test(h)
		}

		return false, nil
	}

	mock.LoadFn = func(h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
		if h.Type == restic.ConfigFile || h.Type == restic.KeyFile {
			return be.Load(h, length, offset)
		}

		return nil, errors.New("not found")
	}

	mock.SaveFn = func(h restic.Handle, rd io.Reader) error {
		if h.Type == restic.ConfigFile || h.Type == restic.KeyFile {
			return be.Save(h, rd)
		}

		return nil
	}

	mock.StatFn = func(h restic.Handle) (restic.FileInfo, error) {
		if h.Type == restic.ConfigFile || h.Type == restic.KeyFile {
			return be.Stat(h)
		}

		return restic.FileInfo{}, errors.New("not found")
	}

	mock.RemoveFn = func(h restic.Handle) error {
		if h.Type == restic.ConfigFile || h.Type == restic.KeyFile {
			return be.Remove(h)
		}

		return nil
	}

	mock.ListFn = func(t restic.FileType, done <-chan struct{}) <-chan string {
		if t == restic.ConfigFile || t == restic.KeyFile {
			return be.List(t, done)
		}

		ch := make(chan string)
		close(ch)
		return ch
	}

	mock.DeleteFn = func() error {
		return nil
	}

	return mock
}

func testArchiverDuplication(t *testing.T) {
	_, err := io.ReadFull(rand.Reader, DupID[:])
	if err != nil {
		t.Fatal(err)
	}

	be := forgetfulBackend()
	if err := repository.Init(be, "foo"); err != nil {
		t.Fatal(err)
	}

	repo, err := repository.Open(be, "foo", 1)
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

	err = repo.Flush()
	if err != nil {
		t.Fatal(err)
	}
}

func TestArchiverDuplication(t *testing.T) {
	for i := 0; i < 5; i++ {
		testArchiverDuplication(t)
	}
}
