package repository

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sort"

	"github.com/klauspost/compress/zstd"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/hashing"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
)

// ErrPackData is returned if errors are discovered while verifying a packfile
type ErrPackData struct {
	PackID restic.ID
	errs   []error
}

func (e *ErrPackData) Error() string {
	return fmt.Sprintf("pack %v contains %v errors: %v", e.PackID, len(e.errs), e.errs)
}

type partialReadError struct {
	err error
}

func (e *partialReadError) Error() string {
	return e.err.Error()
}

// CheckPack reads a pack and checks the integrity of all blobs.
func CheckPack(ctx context.Context, r *Repository, id restic.ID, blobs []restic.Blob, size int64, bufRd *bufio.Reader, dec *zstd.Decoder) error {
	err := checkPackInner(ctx, r, id, blobs, size, bufRd, dec)
	if err != nil {
		if r.Cache != nil {
			// ignore error as there's not much we can do here
			_ = r.Cache.Forget(backend.Handle{Type: restic.PackFile, Name: id.String()})
		}

		// retry pack verification to detect transient errors
		err2 := checkPackInner(ctx, r, id, blobs, size, bufRd, dec)
		if err2 != nil {
			err = err2
		} else {
			err = fmt.Errorf("check successful on second attempt, original error %w", err)
		}
	}
	return err
}

func checkPackInner(ctx context.Context, r *Repository, id restic.ID, blobs []restic.Blob, size int64, bufRd *bufio.Reader, dec *zstd.Decoder) error {

	debug.Log("checking pack %v", id.String())

	if len(blobs) == 0 {
		return &ErrPackData{PackID: id, errs: []error{errors.New("pack is empty or not indexed")}}
	}

	// sanity check blobs in index
	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Offset < blobs[j].Offset
	})
	idxHdrSize := pack.CalculateHeaderSize(blobs)
	lastBlobEnd := 0
	nonContinuousPack := false
	for _, blob := range blobs {
		if lastBlobEnd != int(blob.Offset) {
			nonContinuousPack = true
		}
		lastBlobEnd = int(blob.Offset + blob.Length)
	}
	// size was calculated by masterindex.PackSize, thus there's no need to recalculate it here

	var errs []error
	if nonContinuousPack {
		debug.Log("Index for pack contains gaps / overlaps, blobs: %v", blobs)
		errs = append(errs, errors.New("index for pack contains gaps / overlapping blobs"))
	}

	// calculate hash on-the-fly while reading the pack and capture pack header
	var hash restic.ID
	var hdrBuf []byte
	h := backend.Handle{Type: backend.PackFile, Name: id.String()}
	err := r.be.Load(ctx, h, int(size), 0, func(rd io.Reader) error {
		hrd := hashing.NewReader(rd, sha256.New())
		bufRd.Reset(hrd)

		it := newPackBlobIterator(id, newBufReader(bufRd), 0, blobs, r.Key(), dec)
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			val, err := it.Next()
			if err == errPackEOF {
				break
			} else if err != nil {
				return &partialReadError{err}
			}
			debug.Log("  check blob %v: %v", val.Handle.ID, val.Handle)
			if val.Err != nil {
				debug.Log("  error verifying blob %v: %v", val.Handle.ID, val.Err)
				errs = append(errs, errors.Errorf("blob %v: %v", val.Handle.ID, val.Err))
			}
		}

		// skip enough bytes until we reach the possible header start
		curPos := lastBlobEnd
		minHdrStart := int(size) - pack.MaxHeaderSize
		if minHdrStart > curPos {
			_, err := bufRd.Discard(minHdrStart - curPos)
			if err != nil {
				return &partialReadError{err}
			}
			curPos += minHdrStart - curPos
		}

		// read remainder, which should be the pack header
		var err error
		hdrBuf = make([]byte, int(size-int64(curPos)))
		_, err = io.ReadFull(bufRd, hdrBuf)
		if err != nil {
			return &partialReadError{err}
		}

		hash = restic.IDFromHash(hrd.Sum(nil))
		return nil
	})
	if err != nil {
		var e *partialReadError
		isPartialReadError := errors.As(err, &e)
		// failed to load the pack file, return as further checks cannot succeed anyways
		debug.Log("  error streaming pack (partial %v): %v", isPartialReadError, err)
		if isPartialReadError {
			return &ErrPackData{PackID: id, errs: append(errs, fmt.Errorf("partial download error: %w", err))}
		}

		// The check command suggests to repair files for which a `ErrPackData` is returned. However, this file
		// completely failed to download such that there's no point in repairing anything.
		return fmt.Errorf("download error: %w", err)
	}
	if !hash.Equal(id) {
		debug.Log("pack ID does not match, want %v, got %v", id, hash)
		return &ErrPackData{PackID: id, errs: append(errs, errors.Errorf("unexpected pack id %v", hash))}
	}

	blobs, hdrSize, err := pack.List(r.Key(), bytes.NewReader(hdrBuf), int64(len(hdrBuf)))
	if err != nil {
		return &ErrPackData{PackID: id, errs: append(errs, err)}
	}

	if uint32(idxHdrSize) != hdrSize {
		debug.Log("Pack header size does not match, want %v, got %v", idxHdrSize, hdrSize)
		errs = append(errs, errors.Errorf("pack header size does not match, want %v, got %v", idxHdrSize, hdrSize))
	}

	for _, blob := range blobs {
		// Check if blob is contained in index and position is correct
		idxHas := false
		for _, pb := range r.LookupBlob(blob.BlobHandle.Type, blob.BlobHandle.ID) {
			if pb.PackID == id && pb.Blob == blob {
				idxHas = true
				break
			}
		}
		if !idxHas {
			errs = append(errs, errors.Errorf("blob %v is not contained in index or position is incorrect", blob.ID))
			continue
		}
	}

	if len(errs) > 0 {
		return &ErrPackData{PackID: id, errs: errs}
	}

	return nil
}

type bufReader struct {
	rd  *bufio.Reader
	buf []byte
}

func newBufReader(rd *bufio.Reader) *bufReader {
	return &bufReader{
		rd: rd,
	}
}

func (b *bufReader) Discard(n int) (discarded int, err error) {
	return b.rd.Discard(n)
}

func (b *bufReader) ReadFull(n int) (buf []byte, err error) {
	if cap(b.buf) < n {
		b.buf = make([]byte, n)
	}
	b.buf = b.buf[:n]

	_, err = io.ReadFull(b.rd, b.buf)
	if err != nil {
		return nil, err
	}
	return b.buf, nil
}
