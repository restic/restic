package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/cenkalti/backoff/v4"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// LoadRaw reads all data stored in the backend for the file with id and filetype t.
// If the backend returns data that does not match the id, then the buffer is returned
// along with an error that is a restic.ErrInvalidData error.
func (r *Repository) LoadRaw(ctx context.Context, t restic.FileType, id restic.ID) (buf []byte, err error) {
	h := backend.Handle{Type: backend.FileType(t), Name: id.String()}

	buf, err = loadRaw(ctx, r.be, h)

	// retry loading damaged data only once. If a file fails to download correctly
	// the second time, then it is likely corrupted at the backend.
	if h.Type != backend.ConfigFile && id != restic.Hash(buf) {
		if r.cache != nil {
			// Cleanup cache to make sure it's not the cached copy that is broken.
			// Ignore error as there's not much we can do in that case.
			_ = r.cache.Forget(h)
		}

		buf, err = loadRaw(ctx, r.be, h)

		if err == nil && id != restic.Hash(buf) {
			// Return corrupted data to the caller if it is still broken the second time to
			// let the caller decide what to do with the data.
			return buf, fmt.Errorf("LoadRaw(%v): %w", h, restic.ErrInvalidData)
		}
	}

	if err != nil {
		return nil, err
	}
	return buf, nil
}

// capBuffer stores at most `remain` bytes. We can't use io.LimitReader as it would hide WriteTo,
// which is required for performance reasons on some backends.
type capBuffer struct {
	buf    bytes.Buffer
	remain int
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if len(p) > c.remain {
		n, _ := c.buf.Write(p[:c.remain])
		c.remain -= n
		return n, errors.New("file exceeds maximum size")
	}
	n, _ := c.buf.Write(p)
	c.remain -= n
	return n, nil
}

func loadRaw(ctx context.Context, be backend.Backend, h backend.Handle) (buf []byte, err error) {
	limit := smallFileSizeLimit(h.Type)
	// limit cannot be passed directly to `be.Load` as that would reject a shorter file.
	// Instead, bound the copy operation.
	err = be.Load(ctx, h, 0, 0, func(rd io.Reader) error {
		if limit == 0 {
			wr := new(bytes.Buffer)
			if _, cerr := io.Copy(wr, rd); cerr != nil {
				return cerr
			}
			buf = wr.Bytes()
			return nil
		}

		wr := &capBuffer{remain: limit + 1}
		_, cerr := io.Copy(wr, rd)
		if wr.buf.Len() > limit {
			return backoff.Permanent(errors.Errorf("%s file exceeds maximum size of %d bytes", h.Type, limit))
		}
		if cerr != nil {
			return cerr
		}
		buf = wr.buf.Bytes()
		return nil
	})
	return buf, err
}

func smallFileSizeLimit(t backend.FileType) int {
	switch t {
	case backend.KeyFile, backend.LockFile, backend.ConfigFile:
		// Files are expected to be a few kilobytes. Use a generous limit of 1MB.
		return 1 << 20
	case backend.IndexFile:
		// In the worst case, a JSON encoded index entry can require nearly 256bytes (one blob per pack).
		// An index is split after reaching 50000 blobs. As a pack file is always stored in full, account
		// for the maximum (16MB pack header size limit, 37 bytes per uncompressed blob). Historically,
		// compressed indexes could contain more blobs, but as we work with compressed bytes here, this
		// is already accounted for. This results in the following generous overapproximation:
		// 256 * (50*1000 + 16*1024*1024/37) ~= 123MB
		return 128 * (1 << 20)
	default:
		// Snapshots currently have no size limit as they can include arbitrary amounts of exclude paths.
		return 0
	}
}
