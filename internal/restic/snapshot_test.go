package restic_test

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestNewSnapshot(t *testing.T) {
	paths := []string{"/home/foobar"}

	_, err := restic.NewSnapshot(paths, nil, "foo", time.Now(), "", 0)
	rtest.OK(t, err)
}
