package main

import (
	"fmt"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestPreventNegativeForgetOptionValues(t *testing.T) {
	invalidForgetOpts := []ForgetOptions{
		{Last: -2},
		{Hourly: -2},
		{Daily: -2},
		{Weekly: -2},
		{Monthly: -2},
		{Yearly: -2},
	}

	for _, opts := range invalidForgetOpts {
		err := verifyForgetOptions(&opts)
		rtest.Assert(t, err != nil, fmt.Sprintf("should have returned error for %+v", opts))
		rtest.Equals(t, "Fatal: negative values other than -1 are not allowed for --keep-* options", err.Error())
	}
}
