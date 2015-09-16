package backend_test

import (
	"testing"

	"github.com/restic/restic/backend/gcs"
)

func TestGCSBackend(t *testing.T) {
	s := gcs.OpenMemory()
	testBackend(s, t)
}
