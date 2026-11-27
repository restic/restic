package repository_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/cache"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/backend/mock"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

const KiB = 1 << 10
const MiB = 1 << 20

func TestLoadRaw(t *testing.T) {
	b := mem.New()
	repo, err := repository.New(b, repository.Options{})
	rtest.OK(t, err)

	for i := range 5 {
		data := rtest.Random(23+i, 500*KiB)

		id := restic.Hash(data)
		h := backend.Handle{Name: id.String(), Type: backend.PackFile}
		err := b.Save(context.TODO(), h, backend.NewByteReader(data, b.Hasher()))
		rtest.OK(t, err)

		buf, err := repo.LoadRaw(context.TODO(), restic.PackFile, id)
		rtest.OK(t, err)

		if len(buf) != len(data) {
			t.Errorf("length of returned buffer does not match, want %d, got %d", len(data), len(buf))
			continue
		}

		if !bytes.Equal(buf, data) {
			t.Errorf("wrong data returned")
			continue
		}
	}
}

func TestLoadRawLimitsSmallFiles(t *testing.T) {
	over := rtest.Random(1, MiB+1)
	exact := over[:MiB]

	cases := []struct {
		name string
		tpe  restic.FileType
		data []byte
		ok   bool
	}{
		{"key at limit", restic.KeyFile, exact, true},
		{"key over limit", restic.KeyFile, over, false},
		{"lock over limit", restic.LockFile, over, false},
		{"config over limit", restic.ConfigFile, over, false},
		{"snapshot over limit", restic.SnapshotFile, over, true},
		// only check that 1MB indexes are loaded, the full limit would slow down tests
		{"index over limit", restic.IndexFile, over, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := mem.New()
			repo, err := repository.New(b, repository.Options{})
			rtest.OK(t, err)

			id := restic.Hash(tc.data)
			h := backend.Handle{Type: backend.FileType(tc.tpe), Name: id.String()}
			rtest.OK(t, b.Save(context.TODO(), h, backend.NewByteReader(tc.data, b.Hasher())))

			buf, err := repo.LoadRaw(context.TODO(), tc.tpe, id)
			if tc.ok {
				rtest.OK(t, err)
				rtest.Assert(t, bytes.Equal(buf, tc.data), "wrong data returned for %s", tc.tpe)
				return
			}
			rtest.Assert(t, err != nil && bytes.Contains([]byte(err.Error()), []byte("exceeds maximum size")),
				"expected a size limit error for %s, got %v", tc.tpe, err)
			rtest.Assert(t, buf == nil, "oversized %s returned %d bytes", tc.tpe, len(buf))
		})
	}
}

type writeToOnly struct {
	rd io.Reader
}

func (r *writeToOnly) Read([]byte) (int, error) {
	return 0, errors.New("should have called WriteTo")
}

func (r *writeToOnly) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, r.rd)
}

func (r *writeToOnly) Close() error { return nil }

func TestLoadRawUsesWriteTo(t *testing.T) {
	data := []byte("key file")
	id := restic.Hash(data)
	b := mock.NewBackend()
	b.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		return &writeToOnly{rd: bytes.NewReader(data)}, nil
	}
	repo, err := repository.New(b, repository.Options{})
	rtest.OK(t, err)

	buf, err := repo.LoadRaw(context.TODO(), restic.KeyFile, id)
	rtest.OK(t, err)
	rtest.Equals(t, data, buf)
}

func TestLoadRawBroken(t *testing.T) {
	b := mock.NewBackend()
	repo, err := repository.New(b, repository.Options{})
	rtest.OK(t, err)

	data := rtest.Random(23, 10*KiB)
	id := restic.Hash(data)
	// damage buffer
	data[0] ^= 0xff

	b.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	// must detect but still return corrupt data
	buf, err := repo.LoadRaw(context.TODO(), restic.PackFile, id)
	rtest.Assert(t, bytes.Equal(buf, data), "wrong data returned")
	rtest.Assert(t, errors.Is(err, restic.ErrInvalidData), "missing expected ErrInvalidData error, got %v", err)

	// cause the first access to fail, but repair the data for the second access
	data[0] ^= 0xff
	loadCtr := 0
	b.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		data[0] ^= 0xff
		loadCtr++
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	// must retry load of corrupted data
	buf, err = repo.LoadRaw(context.TODO(), restic.PackFile, id)
	rtest.OK(t, err)
	rtest.Assert(t, bytes.Equal(buf, data), "wrong data returned")
	rtest.Equals(t, 2, loadCtr, "missing retry on broken data")
}

func TestLoadRawBrokenWithCache(t *testing.T) {
	b := mock.NewBackend()
	c := cache.TestNewCache(t)
	repo, err := repository.New(b, repository.Options{})
	rtest.OK(t, err)
	repo.UseCache(c, t.Logf)

	data := rtest.Random(23, 10*KiB)
	id := restic.Hash(data)

	loadCtr := 0
	// cause the first access to fail, but repair the data for the second access
	b.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		data[0] ^= 0xff
		loadCtr++
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	// must retry load of corrupted data
	buf, err := repo.LoadRaw(context.TODO(), restic.SnapshotFile, id)
	rtest.OK(t, err)
	rtest.Assert(t, bytes.Equal(buf, data), "wrong data returned")
	rtest.Equals(t, 2, loadCtr, "missing retry on broken data")
}
