package restic_test

import (
	"testing"

	"github.com/restic/restic"
	. "github.com/restic/restic/test"
)

func TestNewSnapshot(t *testing.T) {
	paths := []string{"/home/foobar"}

	_, err := restic.NewSnapshot(paths)
	OK(t, err)
}
