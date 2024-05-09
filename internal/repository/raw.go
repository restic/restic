package repository

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// LoadRaw reads all data stored in the backend for the file with id and filetype t.
// If the backend returns data that does not match the id, then the buffer is returned
// along with an error that is a restic.ErrInvalidData error.
func (r *Repository) LoadRaw(ctx context.Context, t restic.FileType, id restic.ID) (buf []byte, err error) {
	h := backend.Handle{Type: t, Name: id.String()}

	ctx, cancel := context.WithCancel(ctx)

	var dataErr error
	retriedInvalidData := false
	err = r.be.Load(ctx, h, 0, 0, func(rd io.Reader) error {
		// make sure this is idempotent, in case an error occurs this function may be called multiple times!
		wr := bytes.NewBuffer(buf[:0])
		_, cerr := io.Copy(wr, rd)
		if cerr != nil {
			return cerr
		}
		buf = wr.Bytes()

		// retry loading damaged data only once. If a file fails to download correctly
		// the second time, then it  is likely corrupted at the backend.
		if h.Type != backend.ConfigFile {
			if id != restic.Hash(buf) {
				if !retriedInvalidData {
					debug.Log("retry loading broken blob %v", h)
					retriedInvalidData = true
				} else {
					// with a canceled context there is not guarantee which error will
					// be returned by `be.Load`.
					dataErr = fmt.Errorf("loadAll(%v): %w", h, restic.ErrInvalidData)
					cancel()
				}
				return restic.ErrInvalidData
			}
		}
		return nil
	})

	// Return corrupted data to the caller if it is still broken the second time to
	// let the caller decide what to do with the data.
	if dataErr != nil {
		return buf, dataErr
	}

	if err != nil {
		return nil, err
	}

	return buf, nil
}
