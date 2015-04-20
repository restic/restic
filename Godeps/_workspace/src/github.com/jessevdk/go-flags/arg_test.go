package flags

import (
	"testing"
)

func TestPositional(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Positional struct {
			Command  int
			Filename string
			Rest     []string
		} `positional-args:"yes" required:"yes"`
	}{}

	p := NewParser(&opts, Default)
	ret, err := p.ParseArgs([]string{"10", "arg_test.go", "a", "b"})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
		return
	}

	if opts.Positional.Command != 10 {
		t.Fatalf("Expected opts.Positional.Command to be 10, but got %v", opts.Positional.Command)
	}

	if opts.Positional.Filename != "arg_test.go" {
		t.Fatalf("Expected opts.Positional.Filename to be \"arg_test.go\", but got %v", opts.Positional.Filename)
	}

	assertStringArray(t, opts.Positional.Rest, []string{"a", "b"})
	assertStringArray(t, ret, []string{})
}

func TestPositionalRequired(t *testing.T) {
	var opts = struct {
		Value bool `short:"v"`

		Positional struct {
			Command  int
			Filename string
			Rest     []string
		} `positional-args:"yes" required:"yes"`
	}{}

	p := NewParser(&opts, None)
	_, err := p.ParseArgs([]string{"10"})

	assertError(t, err, ErrRequired, "the required argument `Filename` was not provided")
}
