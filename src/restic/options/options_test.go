package options

import (
	"fmt"
	"reflect"
	"testing"
)

var optsTests = []struct {
	input  []string
	output Options
}{
	{
		[]string{"foo=bar", "bar=baz ", "k="},
		Options{
			"foo": "bar",
			"bar": "baz",
			"k":   "",
		},
	},
	{
		[]string{"Foo=23", "baR", "k=thing with spaces"},
		Options{
			"foo": "23",
			"bar": "",
			"k":   "thing with spaces",
		},
	},
	{
		[]string{"k=thing with spaces", "k2=more spaces = not evil"},
		Options{
			"k":  "thing with spaces",
			"k2": "more spaces = not evil",
		},
	},
	{
		[]string{"x=1", "foo=bar", "y=2", "foo=bar"},
		Options{
			"x":   "1",
			"y":   "2",
			"foo": "bar",
		},
	},
}

func TestParseOptions(t *testing.T) {
	for i, test := range optsTests {
		t.Run(fmt.Sprintf("test-%v", i), func(t *testing.T) {
			opts, err := Parse(test.input)
			if err != nil {
				t.Fatalf("unable to parse options: %v", err)
			}

			if !reflect.DeepEqual(opts, test.output) {
				t.Fatalf("wrong result, want:\n  %#v\ngot:\n  %#v", test.output, opts)
			}
		})
	}
}

var invalidOptsTests = []struct {
	input []string
	err   string
}{
	{
		[]string{"=bar", "bar=baz", "k="},
		"empty key is not a valid option",
	},
	{
		[]string{"x=1", "foo=bar", "y=2", "foo=baz"},
		`key "foo" present more than once`,
	},
}

func TestParseInvalidOptions(t *testing.T) {
	for _, test := range invalidOptsTests {
		t.Run(test.err, func(t *testing.T) {
			_, err := Parse(test.input)
			if err == nil {
				t.Fatalf("expected error (%v) not found, err is nil", test.err)
			}

			if err.Error() != test.err {
				t.Fatalf("expected error %q, got %q", test.err, err.Error())
			}
		})
	}
}

var extractTests = []struct {
	input  Options
	ns     string
	output Options
}{
	{
		input: Options{
			"foo.bar:":     "baz",
			"s3.timeout":   "10s",
			"sftp.timeout": "5s",
			"global":       "foobar",
		},
		ns: "s3",
		output: Options{
			"timeout": "10s",
		},
	},
}

func TestOptionsExtract(t *testing.T) {
	for _, test := range extractTests {
		t.Run(test.ns, func(t *testing.T) {
			opts := test.input.Extract(test.ns)

			if !reflect.DeepEqual(opts, test.output) {
				t.Fatalf("wrong result, want:\n  %#v\ngot:\n  %#v", test.output, opts)
			}
		})
	}
}
