package backend_test

import (
	"testing"

	"github.com/restic/restic/backend"
)

func TestMemoryBackend(t *testing.T) {
	be := backend.NewMemoryBackend()
	testBackend(be, t)
}
