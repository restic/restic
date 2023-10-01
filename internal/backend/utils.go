package backend

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/minio/sha256-simd"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

func verifyContentMatchesName(s string, data []byte) (bool, error) {
	if len(s) != hex.EncodedLen(sha256.Size) {
		return false, fmt.Errorf("invalid length for ID: %q", s)
	}

	b, err := hex.DecodeString(s)
	if err != nil {
		return false, fmt.Errorf("invalid ID: %s", err)
	}
	var id [sha256.Size]byte
	copy(id[:], b)

	hashed := sha256.Sum256(data)
	return id == hashed, nil
}

// LoadAll reads all data stored in the backend for the handle into the given
// buffer, which is truncated. If the buffer is not large enough or nil, a new
// one is allocated.
func LoadAll(ctx context.Context, buf []byte, be Backend, h Handle) ([]byte, error) {
	retriedInvalidData := false
	err := be.Load(ctx, h, 0, 0, func(rd io.Reader) error {
		// make sure this is idempotent, in case an error occurs this function may be called multiple times!
		wr := bytes.NewBuffer(buf[:0])
		_, cerr := io.Copy(wr, rd)
		if cerr != nil {
			return cerr
		}
		buf = wr.Bytes()

		// retry loading damaged data only once. If a file fails to download correctly
		// the second time, then it  is likely corrupted at the backend. Return the data
		// to the caller in that case to let it decide what to do with the data.
		if !retriedInvalidData && h.Type != ConfigFile {
			if matches, err := verifyContentMatchesName(h.Name, buf); err == nil && !matches {
				debug.Log("retry loading broken blob %v", h)
				retriedInvalidData = true
				return errors.Errorf("loadAll(%v): invalid data returned", h)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return buf, nil
}

// LimitedReadCloser wraps io.LimitedReader and exposes the Close() method.
type LimitedReadCloser struct {
	io.Closer
	io.LimitedReader
}

// LimitReadCloser returns a new reader wraps r in an io.LimitedReader, but also
// exposes the Close() method.
func LimitReadCloser(r io.ReadCloser, n int64) *LimitedReadCloser {
	return &LimitedReadCloser{Closer: r, LimitedReader: io.LimitedReader{R: r, N: n}}
}
