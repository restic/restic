package backend

import (
	"reflect"
	"testing"
)

func TestShellSplitter(t *testing.T) {
	var tests = []struct {
		data string
		args []string
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
			`'bar box' baz`,
			[]string{"bar box", "baz"},
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
			args, err := SplitShellStrings(test.data)
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(args, test.args) {
				t.Fatalf("wrong args returned, want:\n  %#v\ngot:\n  %#v",
					test.args, args)
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
			args, err := SplitShellStrings(test.data)
			if err == nil {
				t.Fatalf("expected error not found: %v", test.err)
			}

			if err.Error() != test.err {
				t.Fatalf("expected error not found, want:\n  %q\ngot:\n  %q", test.err, err.Error())
			}

			if len(args) > 0 {
				t.Fatalf("splitter returned fields from invalid data: %v", args)
			}
		})
	}
}
