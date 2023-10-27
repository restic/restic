package backend_test

import (
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/test"
)

type testBackend struct {
	backend.Backend
}

func (t *testBackend) Unwrap() backend.Backend {
	return nil
}

type otherTestBackend struct {
	backend.Backend
}

func (t *otherTestBackend) Unwrap() backend.Backend {
	return t.Backend
}

func TestAsBackend(t *testing.T) {
	other := otherTestBackend{}
	test.Assert(t, backend.AsBackend[*testBackend](other) == nil, "otherTestBackend is not a testBackend backend")

	testBe := &testBackend{}
	test.Assert(t, backend.AsBackend[*testBackend](testBe) == testBe, "testBackend was not returned")

	wrapper := &otherTestBackend{Backend: testBe}
	test.Assert(t, backend.AsBackend[*testBackend](wrapper) == testBe, "failed to unwrap testBackend backend")

	wrapper.Backend = other
	test.Assert(t, backend.AsBackend[*testBackend](wrapper) == nil, "a wrapped otherTestBackend is not a testBackend")
}
