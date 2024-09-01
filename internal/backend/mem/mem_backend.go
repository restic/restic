package mem

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"net/http"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

type memMap map[backend.Handle][]byte

// make sure that MemoryBackend implements backend.Backend
var _ backend.Backend = &MemoryBackend{}

// NewFactory creates a persistent mem backend
func NewFactory() location.Factory {
	be := New()

	return location.NewHTTPBackendFactory[struct{}, *MemoryBackend](
		"mem",
		func(_ string) (*struct{}, error) {
			return &struct{}{}, nil
		},
		location.NoPassword,
		func(_ context.Context, _ struct{}, _ http.RoundTripper) (*MemoryBackend, error) {
			return be, nil
		},
		func(_ context.Context, _ struct{}, _ http.RoundTripper) (*MemoryBackend, error) {
			return be, nil
		},
	)
}

var errNotFound = fmt.Errorf("not found")
var errTooSmall = errors.New("access beyond end of file")

const connectionCount = 2

// MemoryBackend is a mock backend that uses a map for storing all data in
// memory. This should only be used for tests.
type MemoryBackend struct {
	data memMap
	m    sync.Mutex
}

// New returns a new backend that saves all data in a map in memory.
func New() *MemoryBackend {
	be := &MemoryBackend{
		data: make(memMap),
	}

	debug.Log("created new memory backend")

	return be
}

// IsNotExist returns true if the file does not exist.
func (be *MemoryBackend) IsNotExist(err error) bool {
	return errors.Is(err, errNotFound)
}

func (be *MemoryBackend) IsPermanentError(err error) bool {
	return be.IsNotExist(err) || errors.Is(err, errTooSmall)
}

// Save adds new Data to the backend.
func (be *MemoryBackend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	be.m.Lock()
	defer be.m.Unlock()

	h.IsMetadata = false
	if h.Type == backend.ConfigFile {
		h.Name = ""
	}

	if _, ok := be.data[h]; ok {
		return errors.New("file already exists")
	}

	buf, err := io.ReadAll(rd)
	if err != nil {
		return err
	}

	// sanity check
	if int64(len(buf)) != rd.Length() {
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", len(buf), rd.Length())
	}

	beHash := be.Hasher()
	// must never fail according to interface
	_, err = beHash.Write(buf)
	if err != nil {
		panic(err)
	}
	if !bytes.Equal(beHash.Sum(nil), rd.Hash()) {
		return errors.Errorf("invalid file hash or content, got %s expected %s",
			base64.RawStdEncoding.EncodeToString(beHash.Sum(nil)),
			base64.RawStdEncoding.EncodeToString(rd.Hash()),
		)
	}

	be.data[h] = buf

	return ctx.Err()
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *MemoryBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return util.DefaultLoad(ctx, h, length, offset, be.openReader, fn)
}

func (be *MemoryBackend) openReader(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	be.m.Lock()
	defer be.m.Unlock()

	h.IsMetadata = false
	if h.Type == backend.ConfigFile {
		h.Name = ""
	}

	if _, ok := be.data[h]; !ok {
		return nil, errNotFound
	}

	buf := be.data[h]
	if offset+int64(length) > int64(len(buf)) {
		return nil, errTooSmall
	}

	buf = buf[offset:]
	if length > 0 {
		buf = buf[:length]
	}

	return io.NopCloser(bytes.NewReader(buf)), ctx.Err()
}

// Stat returns information about a file in the backend.
func (be *MemoryBackend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	be.m.Lock()
	defer be.m.Unlock()

	h.IsMetadata = false
	if h.Type == backend.ConfigFile {
		h.Name = ""
	}

	e, ok := be.data[h]
	if !ok {
		return backend.FileInfo{}, errNotFound
	}

	return backend.FileInfo{Size: int64(len(e)), Name: h.Name}, ctx.Err()
}

// Remove deletes a file from the backend.
func (be *MemoryBackend) Remove(ctx context.Context, h backend.Handle) error {
	be.m.Lock()
	defer be.m.Unlock()

	h.IsMetadata = false
	if _, ok := be.data[h]; !ok {
		return errNotFound
	}

	delete(be.data, h)

	return ctx.Err()
}

// List returns a channel which yields entries from the backend.
func (be *MemoryBackend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	entries := make(map[string]int64)

	be.m.Lock()
	for entry, buf := range be.data {
		if entry.Type != t {
			continue
		}

		entries[entry.Name] = int64(len(buf))
	}
	be.m.Unlock()

	for name, size := range entries {
		fi := backend.FileInfo{
			Name: name,
			Size: size,
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn(fi)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return ctx.Err()
}

func (be *MemoryBackend) Connections() uint {
	return connectionCount
}

// Hasher may return a hash function for calculating a content hash for the backend
func (be *MemoryBackend) Hasher() hash.Hash {
	return xxhash.New()
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (be *MemoryBackend) HasAtomicReplace() bool {
	return false
}

// Delete removes all data in the backend.
func (be *MemoryBackend) Delete(ctx context.Context) error {
	be.m.Lock()
	defer be.m.Unlock()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	be.data = make(memMap)
	return nil
}

// Close closes the backend.
func (be *MemoryBackend) Close() error {
	return nil
}
