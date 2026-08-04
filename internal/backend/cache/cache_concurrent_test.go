package cache

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// TestNewConcurrent verifies that several restic processes may initialize the
// same cache directory at the same time without any of them failing.
// Regression test for https://github.com/restic/restic/issues/21940, where a
// concurrent reader could observe the partially-written "version" file.
func TestNewConcurrent(t *testing.T) {
	basedir := filepath.Join(rtest.TempDir(t), "cache")

	for round := 0; round < 20; round++ {
		id := restic.NewRandomID().String()

		const n = 32
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := New(id, basedir); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)

		for err := range errs {
			t.Errorf("round %d: New failed: %v", round, err)
		}
	}
}
