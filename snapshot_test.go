package restic_test

import (
	"testing"

	"github.com/restic/restic"
	. "github.com/restic/restic/test"
)

func TestNewSnapshot(t *testing.T) {
	s := SetupBackend(t)
	defer TeardownBackend(t, s)

	paths := []string{"/home/foobar"}

	_, err := restic.NewSnapshot(paths)
	OK(t, err)
}
