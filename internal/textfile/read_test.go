package textfile

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/restic/restic/internal/fs"
)

func writeTempfile(t testing.TB, data []byte) (fs.File, func()) {
	f, removeTempfile := fs.TestTempFile(t, "restic-test-textfile-read-")

	_, err := f.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	return f, removeTempfile
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

			f, cleanup := writeTempfile(t, test.data)
			defer cleanup()

			data, err := Read(f.Name())
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(want, data) {
				t.Errorf("invalid data returned, want:\n  %q\ngot:\n  %q", want, data)
			}
		})
	}
}
