package repository

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

var checkerTestData = filepath.Join("..", "checker", "testdata", "checker-test-repo.tar.gz")

func testWrapCheckPack(ctx context.Context, t *testing.T, repo *Repository,
	packID restic.ID, blobs pack.Blobs, size int64,
) error {
	t.Helper()
	bufRd := bufio.NewReaderSize(nil, maxStreamBufferSize)
	dec, err := zstd.NewReader(nil)
	rtest.OK(t, err)

	return checkPack(ctx, repo, packID, blobs, size, bufRd, dec)
}

// TestGapInBlobs creates a gap in the blob list by omitting the first entry before passing it to checkPack
func TestGapInBlobs(t *testing.T) {
	repo, _ := TestFromFixture(t, checkerTestData)

	err := repo.LoadIndex(context.TODO(), restic.NoopTerminalCounterFactory)
	rtest.OK(t, err)

	repoPacks, err := pack.Size(context.TODO(), repo, false)
	rtest.OK(t, err)

	packID := restic.TestParseID("19a731a515618ec8b75fc0ff3b887d8feb83aef1001c9899f6702761142ed068")
	_, ok := repoPacks[packID]
	rtest.Assert(t, ok, "expected pack 19a731a515618ec8b75fc0ff3b887d8feb83aef1001c9899f6702761142ed068")

	blobs := pack.Blobs{}
	pb := <-repo.listPacksFromIndex(context.TODO(), restic.NewIDSet(packID))
	blobs = append(blobs, pb.Blobs...)

	// assertion for clarity, actually can't fail as the packfile content is fixed
	rtest.Assert(t, len(blobs) >= 2, "need at least 2 blobs in packfile 19a731a51")
	blobs = blobs[1:]
	err = testWrapCheckPack(context.TODO(), t, repo, packID, blobs, repoPacks[packID])

	var packErr *ErrPackData
	rtest.Assert(t, errors.As(err, &packErr), "expected ErrPackData, got: %T %v", err, err)
	rtest.Equals(t, packID, packErr.PackID)

	errText := err.Error()
	rtest.Assert(t, strings.Contains(errText, "gaps") || strings.Contains(errText, "overlapping"),
		"expected gap/overlap error in: %v", errText)
	rtest.Assert(t, strings.Contains(errText, "pack header size does not match"),
		"expected header size mismatch error in: %v", errText)
}

// helper functions for backend error fails

// collectErrors collects errors from checker methods
func collectErrors(ctx context.Context, f func(context.Context, chan<- error)) (errs []error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errChan := make(chan error)

	go f(ctx, errChan)

	for err := range errChan {
		errs = append(errs, err)
	}

	return errs
}

// runReadPacks calls ReadPacks which loads data from specified packs and checks the integrity
func runReadPacks(chkr *Checker) []error {
	return collectErrors(context.TODO(),
		func(ctx context.Context, errCh chan<- error) {
			chkr.ReadPacks(ctx, func(packs map[restic.ID]int64) map[restic.ID]int64 {
				return packs
			}, restic.NoopCounter, errCh)
		})
}

// lastByteFlipBackend flips the last byte of every pack file on read,
// causing the SHA-256 hash computed by checkPackInner to differ from the
// content-addressed pack ID stored in the filename.
type lastByteFlipBackend struct {
	backend.Backend
}

func (b *lastByteFlipBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	if h.Type != restic.PackFile {
		return b.Backend.Load(ctx, h, length, offset, consumer)
	}
	return b.Backend.Load(ctx, h, length, offset, func(rd io.Reader) error {
		buf, err := io.ReadAll(rd)
		if err != nil {
			return err
		}
		if len(buf) > 0 {
			buf[len(buf)-1] ^= 0xff
		}
		return consumer(bytes.NewReader(buf))
	})
}

// alwaysFailBackend returns a hard, non-partial error for every pack file
// load, simulating a complete download failure (e.g. network unreachable).

type alwaysFailBackend struct {
	backend.Backend
}

func (b *alwaysFailBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	if h.Type == restic.PackFile {
		return errors.New("simulated total download failure")
	}
	return b.Backend.Load(ctx, h, length, offset, consumer)
}

// truncatingBackend returns only the first 8 bytes of every pack file,
// guaranteeing io.ErrUnexpectedEOF inside checkPackInner (a partial read).
type truncatingBackend struct {
	backend.Backend
}

func (b *truncatingBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	if h.Type != restic.PackFile {
		return b.Backend.Load(ctx, h, length, offset, consumer)
	}
	return b.Backend.Load(ctx, h, length, offset, func(rd io.Reader) error {
		buf := make([]byte, 8)
		n, _ := io.ReadFull(rd, buf)
		return consumer(bytes.NewReader(buf[:n]))
	})
}

// setupChecker creates a repository with one snapshot, then re-opens the
// same backend through the given wrapper for use by the checker.
func setupChecker(t *testing.T, wrap func(backend.Backend) backend.Backend) *Checker {
	t.Helper()
	// Write a snapshot into a fresh in-memory repository.
	repo, be := TestRepositoryWithBackend(t, nil, 0, Options{})
	_ = archiver.TestSnapshot(t, repo, ".", nil)

	// Re-open the same backend (now containing real pack files) through
	// the corruption wrapper so the checker reads corrupted data.
	checkRepo := TestOpenBackend(t, wrap(be))
	chkr := newChecker(checkRepo)

	// make sure the index is loaded
	err := checkRepo.LoadIndex(context.TODO(), restic.NoopTerminalCounterFactory)
	rtest.OK(t, err)

	return chkr
}

// TestCheckPackHashMismatch verifies that checkPackInner detects when the
// bytes stored in the backend don't hash to the pack's content-addressed ID.
// Covers the `!hash.Equal(id)` branch → ErrPackData "unexpected pack id".
func TestCheckPackHashMismatch(t *testing.T) {
	chkr := setupChecker(t, func(be backend.Backend) backend.Backend {
		return &lastByteFlipBackend{Backend: be}
	})

	found := false
	dataErrs := runReadPacks(chkr)
	for _, err := range dataErrs {
		if strings.Contains(err.Error(), "unexpected pack id") {
			found = true
		}
	}
	rtest.Assert(t, found, "expected 'unexpected pack id' error, got: %v", dataErrs)
}

// TestCheckPackDownloadError verifies that a complete (non-partial) backend
// load failure is returned as a plain "download error" and NOT as ErrPackData.
func TestCheckPackDownloadError(t *testing.T) {
	chkr := setupChecker(t, func(be backend.Backend) backend.Backend {
		return &alwaysFailBackend{Backend: be}
	})

	dataErrs := runReadPacks(chkr)
	rtest.Assert(t, len(dataErrs) > 0, "expected download errors, got none")

	for _, err := range dataErrs {
		var packErr *ErrPackData
		rtest.Assert(t, !errors.As(err, &packErr),
			"complete download failure must NOT produce ErrPackData, got: %v", err)
		rtest.Assert(t, strings.Contains(err.Error(), "download error"),
			"expected 'download error' in message, got: %v", err)
	}
}

// TestCheckPackPartialDownloadError verifies that a partial read (truncated
// response) is returned as ErrPackData, so the check command can suggest
// `restic repair packs` for the affected pack.
// Covers the partialReadError branch of the be.Load error path.
func TestCheckPackPartialDownloadError(t *testing.T) {
	chkr := setupChecker(t, func(be backend.Backend) backend.Backend {
		return &truncatingBackend{Backend: be}
	})

	dataErrs := runReadPacks(chkr)
	rtest.Assert(t, len(dataErrs) > 0, "expected errors from truncated reads, got none")

	for _, err := range dataErrs {
		var packErr *ErrPackData
		rtest.Assert(t, errors.As(err, &packErr),
			"partial read must produce ErrPackData, got: %T %v", err, err)
	}
}
