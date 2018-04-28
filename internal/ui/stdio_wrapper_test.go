package ui

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStdioWrapper(t *testing.T) {
	var tests = []struct {
		inputs  [][]byte
		outputs []string
	}{
		{
			inputs: [][]byte{
				[]byte("foo"),
			},
			outputs: []string{
				"foo\n",
			},
		},
		{
			inputs: [][]byte{
				[]byte("foo"),
				[]byte("bar"),
				[]byte("\n"),
				[]byte("baz"),
			},
			outputs: []string{
				"foobar\n",
				"baz\n",
			},
		},
		{
			inputs: [][]byte{
				[]byte("foo"),
				[]byte("bar\nbaz\n"),
				[]byte("bump\n"),
			},
			outputs: []string{
				"foobar\n",
				"baz\n",
				"bump\n",
			},
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
			outputs: []string{
				"foobar\n",
				"baz\n",
				"bump\n",
				"xxxz\n",
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			var lines []string
			print := func(s string) {
				lines = append(lines, s)
			}

			w := newLineWriter(print)

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

			if !cmp.Equal(test.outputs, lines) {
				t.Error(cmp.Diff(test.outputs, lines))
			}
		})
	}
}
