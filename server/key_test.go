package server_test

import (
	"testing"

	. "github.com/restic/restic/test"
)

func TestRepo(t *testing.T) {
	s := SetupBackend(t)
	defer TeardownBackend(t, s)
	_ = SetupKey(t, s, TestPassword)
}
