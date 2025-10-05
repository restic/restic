package main

import (
	"testing"

	"github.com/restic/restic/internal/data"
	rtest "github.com/restic/restic/internal/test"
	"github.com/spf13/pflag"
)

func TestForgetPolicyValues(t *testing.T) {
	testCases := []struct {
		input string
		value ForgetPolicyCount
		err   string
	}{
		{"0", ForgetPolicyCount(0), ""},
		{"1", ForgetPolicyCount(1), ""},
		{"unlimited", ForgetPolicyCount(-1), ""},
		{"", ForgetPolicyCount(0), "strconv.ParseInt: parsing \"\": invalid syntax"},
		{"-1", ForgetPolicyCount(0), ErrNegativePolicyCount.Error()},
		{"abc", ForgetPolicyCount(0), "strconv.ParseInt: parsing \"abc\": invalid syntax"},
	}
	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			var count ForgetPolicyCount
			err := count.Set(testCase.input)

			if testCase.err != "" {
				rtest.Assert(t, err != nil, "should have returned error for input %+v", testCase.input)
				rtest.Equals(t, testCase.err, err.Error())
			} else {
				rtest.Assert(t, err == nil, "expected no error for input %+v, got %v", testCase.input, err)
				rtest.Equals(t, testCase.value, count)
				rtest.Equals(t, testCase.input, count.String())
			}
		})
	}
}

func TestForgetOptionValues(t *testing.T) {
	const negValErrorMsg = "Fatal: negative values other than -1 are not allowed for --keep-*"
	const negDurationValErrorMsg = "Fatal: durations containing negative values are not allowed for --keep-within*"
	testCases := []struct {
		input    ForgetOptions
		errorMsg string
	}{
		{ForgetOptions{Last: 1}, ""},
		{ForgetOptions{Hourly: 1}, ""},
		{ForgetOptions{Daily: 1}, ""},
		{ForgetOptions{Weekly: 1}, ""},
		{ForgetOptions{Monthly: 1}, ""},
		{ForgetOptions{Yearly: 1}, ""},
		{ForgetOptions{Last: 0}, ""},
		{ForgetOptions{Hourly: 0}, ""},
		{ForgetOptions{Daily: 0}, ""},
		{ForgetOptions{Weekly: 0}, ""},
		{ForgetOptions{Monthly: 0}, ""},
		{ForgetOptions{Yearly: 0}, ""},
		{ForgetOptions{Last: -1}, ""},
		{ForgetOptions{Hourly: -1}, ""},
		{ForgetOptions{Daily: -1}, ""},
		{ForgetOptions{Weekly: -1}, ""},
		{ForgetOptions{Monthly: -1}, ""},
		{ForgetOptions{Yearly: -1}, ""},
		{ForgetOptions{Last: -2}, negValErrorMsg},
		{ForgetOptions{Hourly: -2}, negValErrorMsg},
		{ForgetOptions{Daily: -2}, negValErrorMsg},
		{ForgetOptions{Weekly: -2}, negValErrorMsg},
		{ForgetOptions{Monthly: -2}, negValErrorMsg},
		{ForgetOptions{Yearly: -2}, negValErrorMsg},
		{ForgetOptions{Within: data.ParseDurationOrPanic("1y2m3d3h")}, ""},
		{ForgetOptions{WithinHourly: data.ParseDurationOrPanic("1y2m3d3h")}, ""},
		{ForgetOptions{WithinDaily: data.ParseDurationOrPanic("1y2m3d3h")}, ""},
		{ForgetOptions{WithinWeekly: data.ParseDurationOrPanic("1y2m3d3h")}, ""},
		{ForgetOptions{WithinMonthly: data.ParseDurationOrPanic("2y4m6d8h")}, ""},
		{ForgetOptions{WithinYearly: data.ParseDurationOrPanic("2y4m6d8h")}, ""},
		{ForgetOptions{Within: data.ParseDurationOrPanic("-1y2m3d3h")}, negDurationValErrorMsg},
		{ForgetOptions{WithinHourly: data.ParseDurationOrPanic("1y-2m3d3h")}, negDurationValErrorMsg},
		{ForgetOptions{WithinDaily: data.ParseDurationOrPanic("1y2m-3d3h")}, negDurationValErrorMsg},
		{ForgetOptions{WithinWeekly: data.ParseDurationOrPanic("1y2m3d-3h")}, negDurationValErrorMsg},
		{ForgetOptions{WithinMonthly: data.ParseDurationOrPanic("-2y4m6d8h")}, negDurationValErrorMsg},
		{ForgetOptions{WithinYearly: data.ParseDurationOrPanic("2y-4m6d8h")}, negDurationValErrorMsg},
	}

	for _, testCase := range testCases {
		err := verifyForgetOptions(&testCase.input)
		if testCase.errorMsg != "" {
			rtest.Assert(t, err != nil, "should have returned error for input %+v", testCase.input)
			rtest.Equals(t, testCase.errorMsg, err.Error())
		} else {
			rtest.Assert(t, err == nil, "expected no error for input %+v", testCase.input)
		}
	}
}

func TestForgetHostnameDefaulting(t *testing.T) {
	t.Setenv("RESTIC_HOST", "testhost")

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "env default when flag not set",
			args: nil,
			want: []string{"testhost"},
		},
		{
			name: "flag overrides env",
			args: []string{"--host", "flaghost"},
			want: []string{"flaghost"},
		},
		{
			name: "empty flag clears env",
			args: []string{"--host", ""},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := pflag.NewFlagSet(tt.name, pflag.ContinueOnError)
			opts := ForgetOptions{}
			opts.AddFlags(set)
			err := set.Parse(tt.args)
			rtest.Assert(t, err == nil, "expected no error for input")
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			rtest.Equals(t, tt.want, opts.Hosts)
		})
	}
}
