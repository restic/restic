package termstatus

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStdioWrapper(t *testing.T) {
	var tests = []struct {
		inputs [][]byte
		output string
	}{
		{
			inputs: [][]byte{
				[]byte("foo"),
			},
			output: "foo\n",
		},
		{
			inputs: [][]byte{
				[]byte("foo"),
				[]byte("bar"),
				[]byte("\n"),
				[]byte("baz"),
			},
			output: "foobar\n" +
				"baz\n",
		},
		{
			inputs: [][]byte{
				[]byte("foo"),
				[]byte("bar\nbaz\n"),
				[]byte("bump\n"),
			},
			output: "foobar\n" +
				"baz\n" +
				"bump\n",
		},
		{
			inputs: [][]byte{
				[]byte("foo"),
				[]byte("bar\nbaz\n"),
				[]byte("bum"),
				[]byte("p\nx"),
				[]byte("x"),
				[]byte("x"),
				[]byte("z"),
			},
			output: "foobar\n" +
				"baz\n" +
				"bump\n" +
				"xxxz\n",
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			var output strings.Builder
			w := newLineWriter(func(s string) { output.WriteString(s) })

			for _, data := range test.inputs {
				n, err := w.Write(data)
				if err != nil {
					t.Fatal(err)
				}

				if n != len(data) {
					t.Errorf("invalid length returned by Write, want %d, got %d", len(data), n)
				}
			}

			err := w.Close()
			if err != nil {
				t.Fatal(err)
			}

			if outstr := output.String(); outstr != test.output {
				t.Error(cmp.Diff(test.output, outstr))
			}
		})
	}
}
