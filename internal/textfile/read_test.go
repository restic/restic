package textfile

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"
)

// writeTempfile writes data to a new temporary file and returns its name
// and a callback that removes it.
func writeTempfile(t testing.TB, data []byte) (string, func()) {
	t.Helper()

	f, err := os.CreateTemp("", "restic-test-textfile-read-")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()

	defer func() {
		closeErr := f.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			// ignore subsequent errors
			_ = os.Remove(name)
			t.Fatal(err)
		}
	}()

	_, err = f.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	return name, func() {
		err := os.Remove(name)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func dec(s string) []byte {
	data, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return data
}

func TestRead(t *testing.T) {
	var tests = []struct {
		data []byte
		want []byte
	}{
		{data: []byte("foo bar baz")},
		{data: []byte("Ööbär")},
		{
			data: []byte("\xef\xbb\xbffööbär"),
			want: []byte("fööbär"),
		},
		{
			data: dec("feff006600f600f6006200e40072"),
			want: []byte("fööbär"),
		},
		{
			data: dec("fffe6600f600f6006200e4007200"),
			want: []byte("fööbär"),
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			want := test.want
			if want == nil {
				want = test.data
			}

			tempname, cleanup := writeTempfile(t, test.data)
			defer cleanup()

			data, err := Read(tempname)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(want, data) {
				t.Errorf("invalid data returned, want:\n  %q\ngot:\n  %q", want, data)
			}
		})
	}
}
