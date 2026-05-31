package repository

import (
	"bufio"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

var checkerTestData = filepath.Join("..", "checker", "testdata", "checker-test-repo.tar.gz")

func testWrapCheckPack(ctx context.Context, t *testing.T, repo *Repository,
	packID restic.ID, blobs []restic.Blob, size int64,
) error {
	t.Helper()
	bufRd := bufio.NewReaderSize(nil, maxStreamBufferSize)
	dec, err := zstd.NewReader(nil)
	test.OK(t, err)
	return CheckPack(ctx, repo, packID, blobs, size, bufRd, dec)
}

func TestGapInBlobs(t *testing.T) {
	repo, _, cleanup := TestFromFixture(t, checkerTestData)
	defer cleanup()

	err := repo.LoadIndex(context.TODO(), nil)
	test.OK(t, err)

	repoPacks, err := pack.Size(context.TODO(), repo, false)
	test.OK(t, err)

	packID := restic.TestParseID("19a731a515618ec8b75fc0ff3b887d8feb83aef1001c9899f6702761142ed068")
	_, ok := repoPacks[packID]
	test.Assert(t, ok, "expected pack 19a731a515618ec8b75fc0ff3b887d8feb83aef1001c9899f6702761142ed068")

	blobs := []restic.Blob{}
	pb := <-repo.ListPacksFromIndex(context.TODO(), restic.NewIDSet(packID))
	blobs = append(blobs, pb.Blobs...)

	// force fail
	test.Assert(t, len(blobs) >= 2, "need at least 2 blobs in packfile 19a731a51")
	blobs = blobs[1:]
	err = testWrapCheckPack(context.TODO(), t, repo, packID, blobs, repoPacks[packID])

	var packErr *ErrPackData
	test.Assert(t, errors.As(err, &packErr), "expected ErrPackData, got: %T %v", err, err)
	test.Equals(t, packID, packErr.PackID)

	errText := err.Error()
	test.Assert(t, strings.Contains(errText, "gaps") || strings.Contains(errText, "overlapping"),
		"expected gap/overlap error in: %v", errText)
	test.Assert(t, strings.Contains(errText, "pack header size does not match"),
		"expected header size mismatch error in: %v", errText)
}
