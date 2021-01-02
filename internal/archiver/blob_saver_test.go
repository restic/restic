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
	idx    restic.MasterIndex
	cnt    int32
	failAt int32
}

func (b *saveFail) SaveBlob(ctx context.Context, t restic.BlobType, buf []byte, id restic.ID, storeDuplicates bool) (restic.ID, bool, error) {
	val := atomic.AddInt32(&b.cnt, 1)
	if val == b.failAt {
		return restic.ID{}, false, errTest
	}

	return id, false, nil
}

func (b *saveFail) Index() restic.MasterIndex {
	return b.idx
}

func TestBlobSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmb, ctx := tomb.WithContext(ctx)
	saver := &saveFail{
		idx: repository.NewMasterIndex(),
	}

	b := NewBlobSaver(ctx, tmb, saver, uint(runtime.NumCPU()))

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

			tmb, ctx := tomb.WithContext(ctx)
			saver := &saveFail{
				idx:    repository.NewMasterIndex(),
				failAt: int32(test.failAt),
			}

			b := NewBlobSaver(ctx, tmb, saver, uint(runtime.NumCPU()))

			for i := 0; i < test.blobs; i++ {
				buf := &Buffer{Data: []byte(fmt.Sprintf("foo%d", i))}
				b.Save(ctx, restic.DataBlob, buf)
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
