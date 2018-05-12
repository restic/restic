package archiver

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	tomb "gopkg.in/tomb.v2"
)

var errTest = errors.New("test error")

type saveFail struct {
	idx    restic.Index
	cnt    int32
	failAt int32
}

func (b *saveFail) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID) (restic.ID, error) {
	val := atomic.AddInt32(&b.cnt, 1)
	if val == b.failAt {
		return restic.ID{}, errTest
	}

	return id, nil
}

func (b *saveFail) Index() restic.Index {
	return b.idx
}

func TestBlobSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var tmb tomb.Tomb
	saver := &saveFail{
		idx: repository.NewIndex(),
	}

	b := NewBlobSaver(ctx, &tmb, saver, uint(runtime.NumCPU()))

	var results []FutureBlob

	for i := 0; i < 20; i++ {
		buf := &Buffer{Data: []byte(fmt.Sprintf("foo%d", i))}
		fb := b.Save(ctx, restic.DataBlob, buf)
		results = append(results, fb)
	}

	for i, blob := range results {
		blob.Wait(ctx)
		if blob.Known() {
			t.Errorf("blob %v is known, that should not be the case", i)
		}
	}

	tmb.Kill(nil)

	err := tmb.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBlobSaverError(t *testing.T) {
	var tests = []struct {
		blobs  int
		failAt int
	}{
		{20, 2},
		{20, 5},
		{20, 15},
		{200, 150},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var tmb tomb.Tomb
			saver := &saveFail{
				idx:    repository.NewIndex(),
				failAt: int32(test.failAt),
			}

			b := NewBlobSaver(ctx, &tmb, saver, uint(runtime.NumCPU()))

			var results []FutureBlob

			for i := 0; i < test.blobs; i++ {
				buf := &Buffer{Data: []byte(fmt.Sprintf("foo%d", i))}
				fb := b.Save(ctx, restic.DataBlob, buf)
				results = append(results, fb)
			}

			tmb.Kill(nil)

			err := tmb.Wait()
			if err == nil {
				t.Errorf("expected error not found")
			}

			if err != errTest {
				t.Fatalf("unexpected error found: %v", err)
			}
		})
	}
}
