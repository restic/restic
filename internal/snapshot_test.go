package restic_test

import (
	"testing"

	"github.com/restic/restic/internal"
	. "github.com/restic/restic/internal/test"
)

func TestNewSnapshot(t *testing.T) {
	paths := []string{"/home/foobar"}

	_, err := restic.NewSnapshot(paths, nil, "foo")
	OK(t, err)
}
