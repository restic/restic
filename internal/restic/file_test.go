package restic

import (
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestHandleString(t *testing.T) {
	rtest.Equals(t, "<data/foobar>", Handle{Type: PackFile, Name: "foobar"}.String())
	rtest.Equals(t, "<lock/1>", Handle{Type: LockFile, Name: "1"}.String())
}

func TestHandleValid(t *testing.T) {
	var handleTests = []struct {
		h     Handle
		valid bool
	}{
		{Handle{Name: "foo"}, false},
		{Handle{Type: 0}, false},
		{Handle{Type: ConfigFile, Name: ""}, true},
		{Handle{Type: PackFile, Name: ""}, false},
		{Handle{Type: LockFile, Name: "010203040506"}, true},
	}

	for i, test := range handleTests {
		err := test.h.Valid()
		if err != nil && test.valid {
			t.Errorf("test %v failed: error returned for valid handle: %v", i, err)
		}

		if !test.valid && err == nil {
			t.Errorf("test %v failed: expected error for invalid handle not found", i)
		}
	}
}
