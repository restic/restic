package restic

import (
	"io/ioutil"
	"testing"
)

func BenchmarkNodeFillUser(t *testing.B) {
	tempfile, err := ioutil.TempFile("", "restic-test-temp-")
	if err != nil {
		t.Fatal(err)
	}

	fi, err := tempfile.Stat()
	if err != nil {
		t.Fatal(err)
	}

	node := &Node{}
	path := tempfile.Name()

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		node.fillExtra(path, fi)
	}
}
