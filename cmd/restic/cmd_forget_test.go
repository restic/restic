package main

import (
	"fmt"
	"testing"

	"github.com/restic/restic/internal/restic"
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
		{Within: restic.Duration{Hours: -2}},
		{Within: restic.Duration{Days: -2}},
		{Within: restic.Duration{Months: -2}},
		{Within: restic.Duration{Years: -2}},
		{WithinHourly: restic.Duration{Hours: -2}},
		{WithinHourly: restic.Duration{Days: -2}},
		{WithinHourly: restic.Duration{Months: -2}},
		{WithinHourly: restic.Duration{Years: -2}},
		{WithinDaily: restic.Duration{Hours: -2}},
		{WithinDaily: restic.Duration{Days: -2}},
		{WithinDaily: restic.Duration{Months: -2}},
		{WithinDaily: restic.Duration{Years: -2}},
		{WithinWeekly: restic.Duration{Hours: -2}},
		{WithinWeekly: restic.Duration{Days: -2}},
		{WithinWeekly: restic.Duration{Months: -2}},
		{WithinWeekly: restic.Duration{Years: -2}},
		{WithinMonthly: restic.Duration{Hours: -2}},
		{WithinMonthly: restic.Duration{Days: -2}},
		{WithinMonthly: restic.Duration{Months: -2}},
		{WithinMonthly: restic.Duration{Years: -2}},
		{WithinYearly: restic.Duration{Hours: -2}},
		{WithinYearly: restic.Duration{Days: -2}},
		{WithinYearly: restic.Duration{Months: -2}},
		{WithinYearly: restic.Duration{Years: -2}},
	}

	for _, opts := range invalidForgetOpts {
		err := verifyForgetOptions(&opts)
		rtest.Assert(t, err != nil, fmt.Sprintf("should have returned error for %+v", opts))
		rtest.Equals(t, "Fatal: negative values other than -1 are not allowed for --keep-* options", err.Error())
	}
}
