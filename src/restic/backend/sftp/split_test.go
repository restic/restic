package sftp

import (
	"reflect"
	"testing"
)

func TestShellSplitter(t *testing.T) {
	var tests = []struct {
		data string
		want []string
	}{
		{
			`foo`,
			[]string{"foo"},
		},
		{
			`'foo'`,
			[]string{"foo"},
		},
		{
			`foo bar baz`,
			[]string{"foo", "bar", "baz"},
		},
		{
			`foo 'bar' baz`,
			[]string{"foo", "bar", "baz"},
		},
		{
			`foo 'bar box' baz`,
			[]string{"foo", "bar box", "baz"},
		},
		{
			`"bar 'box'" baz`,
			[]string{"bar 'box'", "baz"},
		},
		{
			`'bar "box"' baz`,
			[]string{`bar "box"`, "baz"},
		},
		{
			`\"bar box baz`,
			[]string{`"bar`, "box", "baz"},
		},
		{
			`"bar/foo/x" "box baz"`,
			[]string{"bar/foo/x", "box baz"},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			res, err := SplitShellArgs(test.data)
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(res, test.want) {
				t.Fatalf("wrong data returned, want:\n  %#v\ngot:\n  %#v",
					test.want, res)
			}
		})
	}
}

func TestShellSplitterInvalid(t *testing.T) {
	var tests = []struct {
		data string
		err  string
	}{
		{
			"foo'",
			"single-quoted string not terminated",
		},
		{
			`foo"`,
			"double-quoted string not terminated",
		},
		{
			"foo 'bar",
			"single-quoted string not terminated",
		},
		{
			`foo "bar`,
			"double-quoted string not terminated",
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			res, err := SplitShellArgs(test.data)
			if err == nil {
				t.Fatalf("expected error not found: %v", test.err)
			}

			if err.Error() != test.err {
				t.Fatalf("expected error not found, want:\n  %q\ngot:\n  %q", test.err, err.Error())
			}

			if len(res) > 0 {
				t.Fatalf("splitter returned fields from invalid data: %v", res)
			}
		})
	}
}
