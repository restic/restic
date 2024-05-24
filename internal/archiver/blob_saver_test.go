package archiver

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

var errTest = errors.New("test error")

type saveFail struct {
	cnt    int32
	failAt int32
}

func (b *saveFail) SaveBlob(_ context.Context, _ restic.BlobType, _ []byte, id restic.ID, _ bool) (restic.ID, bool, int, error) {
	val := atomic.AddInt32(&b.cnt, 1)
	if val == b.failAt {
		return restic.ID{}, false, 0, errTest
	}

	return id, false, 0, nil
}

func TestBlobSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg, ctx := errgroup.WithContext(ctx)
	saver := &saveFail{}

	b := NewBlobSaver(ctx, wg, saver, uint(runtime.NumCPU()))

	var wait sync.WaitGroup
	var results []SaveBlobResponse
	var lock sync.Mutex

	wait.Add(20)
	for i := 0; i < 20; i++ {
		buf := &Buffer{Data: []byte(fmt.Sprintf("foo%d", i))}
		idx := i
		lock.Lock()
		results = append(results, SaveBlobResponse{})
		lock.Unlock()
		b.Save(ctx, restic.DataBlob, buf, "file", func(res SaveBlobResponse) {
			lock.Lock()
			results[idx] = res
			lock.Unlock()
			wait.Done()
		})
	}

	wait.Wait()
	for i, sbr := range results {
		if sbr.known {
			t.Errorf("blob %v is known, that should not be the case", i)
		}
	}

	b.TriggerShutdown()

	err := wg.Wait()
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

			wg, ctx := errgroup.WithContext(ctx)
			saver := &saveFail{
				failAt: int32(test.failAt),
			}

			b := NewBlobSaver(ctx, wg, saver, uint(runtime.NumCPU()))

			for i := 0; i < test.blobs; i++ {
				buf := &Buffer{Data: []byte(fmt.Sprintf("foo%d", i))}
				b.Save(ctx, restic.DataBlob, buf, "errfile", func(res SaveBlobResponse) {})
			}

			b.TriggerShutdown()

			err := wg.Wait()
			if err == nil {
				t.Errorf("expected error not found")
			}

			rtest.Assert(t, errors.Is(err, errTest), "unexpected error %v", err)
			rtest.Assert(t, strings.Contains(err.Error(), "errfile"), "expected error to contain 'errfile' got: %v", err)
		})
	}
}
