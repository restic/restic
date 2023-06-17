package restic_test

import (
	"testing"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

type testBackend struct {
	restic.Backend
}

func (t *testBackend) Unwrap() restic.Backend {
	return nil
}

type otherTestBackend struct {
	restic.Backend
}

func (t *otherTestBackend) Unwrap() restic.Backend {
	return t.Backend
}

func TestAsBackend(t *testing.T) {
	other := otherTestBackend{}
	test.Assert(t, restic.AsBackend[*testBackend](other) == nil, "otherTestBackend is not a testBackend backend")

	testBe := &testBackend{}
	test.Assert(t, restic.AsBackend[*testBackend](testBe) == testBe, "testBackend was not returned")

	wrapper := &otherTestBackend{Backend: testBe}
	test.Assert(t, restic.AsBackend[*testBackend](wrapper) == testBe, "failed to unwrap testBackend backend")

	wrapper.Backend = other
	test.Assert(t, restic.AsBackend[*testBackend](wrapper) == nil, "a wrapped otherTestBackend is not a testBackend")
}
