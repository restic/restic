package mem_test

import (
	"testing"

	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/backend/test"
)

func newTestSuite() *test.Suite[struct{}] {
	return &test.Suite[struct{}]{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*struct{}, error) {
			return &struct{}{}, nil
		},

		Factory: mem.NewFactory(),
	}
}

func TestSuiteBackendMem(t *testing.T) {
	newTestSuite().RunTests(t)
}

func BenchmarkSuiteBackendMem(t *testing.B) {
	newTestSuite().RunBenchmarks(t)
}
