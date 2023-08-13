package main

import (
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestCatArgsValidation(t *testing.T) {
	for _, test := range []struct {
		args []string
		err  string
	}{
		{[]string{}, "Fatal: type not specified"},
		{[]string{"masterkey"}, ""},
		{[]string{"invalid"}, `Fatal: invalid type "invalid"`},
		{[]string{"snapshot"}, "Fatal: ID not specified"},
		{[]string{"snapshot", "12345678"}, ""},
	} {
		t.Run("", func(t *testing.T) {
			err := validateCatArgs(test.args)
			if test.err == "" {
				rtest.Assert(t, err == nil, "unexpected error %q", err)
			} else {
				rtest.Assert(t, strings.Contains(err.Error(), test.err), "unexpected error expected %q to contain %q", err, test.err)
			}
		})
	}
}
