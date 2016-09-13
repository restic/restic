package restic_test

import (
	"testing"

	"restic"
	. "restic/test"
)

func TestNewSnapshot(t *testing.T) {
	paths := []string{"/home/foobar"}

	_, err := restic.NewSnapshot(paths, nil)
	OK(t, err)
}
