package restic_test

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	. "github.com/restic/restic/internal/test"
)

func TestNewSnapshot(t *testing.T) {
	paths := []string{"/home/foobar"}

	_, err := restic.NewSnapshot(paths, nil, "foo", time.Now())
	OK(t, err)
}
