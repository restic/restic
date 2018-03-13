package backend

import (
	"reflect"
	"testing"
)

func TestShellSplitter(t *testing.T) {
	var tests = []struct {
		data string
		cmd  string
		args []string
	}{
		{
			`foo`,
			"foo", []string{},
		},
		{
			`'foo'`,
			"foo", []string{},
		},
		{
			`foo bar baz`,
			"foo", []string{"bar", "baz"},
		},
		{
			`foo 'bar' baz`,
			"foo", []string{"bar", "baz"},
		},
		{
			`'bar box' baz`,
			"bar box", []string{"baz"},
		},
		{
			`"bar 'box'" baz`,
			"bar 'box'", []string{"baz"},
		},
		{
			`'bar "box"' baz`,
			`bar "box"`, []string{"baz"},
		},
		{
			`\"bar box baz`,
			`"bar`, []string{"box", "baz"},
		},
		{
			`"bar/foo/x" "box baz"`,
			"bar/foo/x", []string{"box baz"},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			cmd, args, err := SplitShellArgs(test.data)
			if err != nil {
				t.Fatal(err)
			}

			if cmd != test.cmd {
				t.Fatalf("wrong cmd returned, want:\n  %#v\ngot:\n  %#v",
					test.cmd, cmd)
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
			cmd, args, err := SplitShellArgs(test.data)
			if err == nil {
				t.Fatalf("expected error not found: %v", test.err)
			}

			if err.Error() != test.err {
				t.Fatalf("expected error not found, want:\n  %q\ngot:\n  %q", test.err, err.Error())
			}

			if cmd != "" {
				t.Fatalf("splitter returned cmd from invalid data: %v", cmd)
			}

			if len(args) > 0 {
				t.Fatalf("splitter returned fields from invalid data: %v", args)
			}
		})
	}
}
