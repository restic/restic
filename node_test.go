package restic

import (
	"io/ioutil"
	"testing"
)

func BenchmarkNodeFillUser(t *testing.B) {
	tempfile, err := ioutil.TempFile("", "restic-test-temp-")
	defer tempfile.Close()
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

func BenchmarkNodeFromFileInfo(t *testing.B) {
	tempfile, err := ioutil.TempFile("", "restic-test-temp-")
	defer tempfile.Close()
	if err != nil {
		t.Fatal(err)
	}

	fi, err := tempfile.Stat()
	if err != nil {
		t.Fatal(err)
	}

	path := tempfile.Name()

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := NodeFromFileInfo(path, fi)
		if err != nil {
			t.Fatal(err)
		}
	}
}
