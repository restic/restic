package restic

import "testing"

var handleTests = []struct {
	h     Handle
	valid bool
}{
	{Handle{Name: "foo"}, false},
	{Handle{Type: "foobar"}, false},
	{Handle{Type: ConfigFile, Name: ""}, true},
	{Handle{Type: PackFile, Name: ""}, false},
	{Handle{Type: "", Name: "x"}, false},
	{Handle{Type: LockFile, Name: "010203040506"}, true},
}

func TestHandleValid(t *testing.T) {
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
