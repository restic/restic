package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestWriteVersionConcurrentReaders(t *testing.T) {
	dir := rtest.TempDir(t)
	rtest.OK(t, writeVersion(dir, cacheVersion))

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				if err := writeVersion(dir, cacheVersion); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	for range 1000 {
		version, err := readVersion(dir)
		rtest.OK(t, err)
		rtest.Equals(t, uint(cacheVersion), version)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		rtest.OK(t, err)
	}
}

func TestWriteVersionExisting(t *testing.T) {
	var tests = []struct {
		name     string
		existing uint
		want     uint
		wantErr  bool
	}{
		{"same", cacheVersion, cacheVersion, false},
		// an older cache is upgraded through the rename fallback
		{"older", cacheVersion - 1, cacheVersion, false},
		// a newer restic won the race, its version must not be downgraded
		{"newer", cacheVersion + 1, cacheVersion + 1, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := rtest.TempDir(t)
			rtest.OK(t, writeVersion(dir, test.existing))

			err := writeVersion(dir, cacheVersion)
			if test.wantErr {
				rtest.Assert(t, err != nil, "expected error for newer cache version")
			} else {
				rtest.OK(t, err)
			}

			version, err := readVersion(dir)
			rtest.OK(t, err)
			rtest.Equals(t, test.want, version)

			tempFiles, err := filepath.Glob(filepath.Join(dir, "version-*"))
			rtest.OK(t, err)
			rtest.Equals(t, 0, len(tempFiles), "temporary version files left behind")
		})
	}
}

func TestNewConcurrent(t *testing.T) {
	basedir := filepath.Join(rtest.TempDir(t), "cache")
	id := restic.NewRandomID().String()

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := New(id, basedir)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		rtest.OK(t, err)
	}
	version, err := readVersion(filepath.Join(basedir, id))
	rtest.OK(t, err)
	rtest.Equals(t, uint(cacheVersion), version)
}

func TestNew(t *testing.T) {
	parent := rtest.TempDir(t)
	basedir := filepath.Join(parent, "cache")
	id := restic.NewRandomID().String()
	tagFile := filepath.Join(basedir, "CACHEDIR.TAG")
	versionFile := filepath.Join(basedir, id, "version")

	const (
		stepCreate = iota
		stepComplete
		stepRmTag
		stepRmVersion
		stepEnd
	)

	for step := range stepEnd {
		switch step {
		case stepRmTag:
			rtest.OK(t, os.Remove(tagFile))
		case stepRmVersion:
			rtest.OK(t, os.Remove(versionFile))
		}

		c, err := New(id, basedir)
		rtest.OK(t, err)
		rtest.Equals(t, basedir, c.Base)
		rtest.Equals(t, step == stepCreate, c.Created)

		for _, name := range []string{tagFile, versionFile} {
			info, err := os.Lstat(name)
			rtest.OK(t, err)
			rtest.Assert(t, info.Mode().IsRegular(), "")
		}
	}
}
