package options

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"
	"time"
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
		"Fatal: empty key is not a valid option",
	},
	{
		[]string{"x=1", "foo=bar", "y=2", "foo=baz"},
		`Fatal: key "foo" present more than once`,
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

// Target is used for Apply() tests
type Target struct {
	Name    string        `option:"name"`
	ID      int           `option:"id"`
	Timeout time.Duration `option:"timeout"`
	Other   string
}

var setTests = []struct {
	input  Options
	output Target
}{
	{
		Options{
			"name": "foobar",
		},
		Target{
			Name: "foobar",
		},
	},
	{
		Options{
			"name": "foobar",
			"id":   "1234",
		},
		Target{
			Name: "foobar",
			ID:   1234,
		},
	},
	{
		Options{
			"timeout": "10m3s",
		},
		Target{
			Timeout: time.Duration(10*time.Minute + 3*time.Second),
		},
	},
}

func TestOptionsApply(t *testing.T) {
	for i, test := range setTests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			var dst Target
			err := test.input.Apply("", &dst)
			if err != nil {
				t.Fatal(err)
			}

			if dst != test.output {
				t.Fatalf("wrong result, want:\n  %#v\ngot:\n  %#v", test.output, dst)
			}
		})
	}
}

var invalidSetTests = []struct {
	input     Options
	namespace string
	err       string
}{
	{
		Options{
			"first_name": "foobar",
		},
		"ns",
		"Fatal: option ns.first_name is not known",
	},
	{
		Options{
			"id": "foobar",
		},
		"ns",
		`strconv.ParseInt: parsing "foobar": invalid syntax`,
	},
	{
		Options{
			"timeout": "2134",
		},
		"ns",
		`time: missing unit in duration "?2134"?`,
	},
}

func TestOptionsApplyInvalid(t *testing.T) {
	for i, test := range invalidSetTests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			var dst Target
			err := test.input.Apply(test.namespace, &dst)
			if err == nil {
				t.Fatalf("expected error %v not found", test.err)
			}

			matched, err := regexp.MatchString(test.err, err.Error())
			if err != nil {
				t.Fatal(err)
			}

			if !matched {
				t.Fatalf("expected error to match %q, got %q", test.err, err.Error())
			}
		})
	}
}

func TestListOptions(t *testing.T) {
	var teststruct = struct {
		Foo string `option:"foo" help:"bar text help"`
	}{}

	var tests = []struct {
		cfg  interface{}
		opts []Help
	}{
		{
			struct {
				Foo string `option:"foo" help:"bar text help"`
			}{},
			[]Help{
				{Name: "foo", Text: "bar text help"},
			},
		},
		{
			struct {
				Foo string `option:"foo" help:"bar text help"`
				Bar string `option:"bar" help:"bar text help"`
			}{},
			[]Help{
				{Name: "foo", Text: "bar text help"},
				{Name: "bar", Text: "bar text help"},
			},
		},
		{
			struct {
				Bar string `option:"bar" help:"bar text help"`
				Foo string `option:"foo" help:"bar text help"`
			}{},
			[]Help{
				{Name: "bar", Text: "bar text help"},
				{Name: "foo", Text: "bar text help"},
			},
		},
		{
			&teststruct,
			[]Help{
				{Name: "foo", Text: "bar text help"},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			opts := listOptions(test.cfg)
			if !reflect.DeepEqual(opts, test.opts) {
				t.Fatalf("wrong opts, want:\n  %v\ngot:\n  %v", test.opts, opts)
			}
		})
	}
}

func TestAppendAllOptions(t *testing.T) {
	var tests = []struct {
		cfgs map[string]interface{}
		opts []Help
	}{
		{
			map[string]interface{}{
				"local": struct {
					Foo string `option:"foo" help:"bar text help"`
				}{},
				"sftp": struct {
					Foo string `option:"foo" help:"bar text help2"`
					Bar string `option:"bar" help:"bar text help"`
				}{},
			},
			[]Help{
				{Namespace: "local", Name: "foo", Text: "bar text help"},
				{Namespace: "sftp", Name: "bar", Text: "bar text help"},
				{Namespace: "sftp", Name: "foo", Text: "bar text help2"},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			var opts []Help
			for ns, cfg := range test.cfgs {
				opts = appendAllOptions(opts, ns, cfg)
			}

			if !reflect.DeepEqual(opts, test.opts) {
				t.Fatalf("wrong list, want:\n  %v\ngot:\n  %v", test.opts, opts)
			}
		})
	}
}
