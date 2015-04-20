// +build !gccgo

package arrar

import (
	gc "launchpad.net/gocheck"
)

func (*internalSuite) TestFunction(c *gc.C) {
	for i, test := range []struct {
		fullFunc  string
		shortFunc string
	}{
		{
			fullFunc:  "github.com/juju/arrar.two",
			shortFunc: "two",
		}, {
			fullFunc:  "github.com/juju/arrar.(*receiver).Func",
			shortFunc: "(*receiver).Func",
		},
	} {
		c.Logf("Test %v", i)
		c.Check(function(test.fullFunc), gc.Equals, test.shortFunc)
	}
}

// For non-gccgo compilers, we expect the function to return the expected test
// result.
func expectedFunctionResult(value string) string {
	return value
}

func (*arrarSuite) TestDetailedErrorStack(c *gc.C) {
	for _, test := range []struct {
		err      error
		expected string
	}{
		{
			err:      one(),
			expected: "one",
		}, {
			err:      two(),
			expected: "two: one [two@github.com/juju/arrar/test_functions_test.go:16]",
		}, {
			err: three(),
			expected: "three [three@github.com/juju/arrar/test_functions_test.go:20]\n" +
				"two: one [two@github.com/juju/arrar/test_functions_test.go:16]",
		}, {
			err: transtwo(),
			expected: "transtwo: translated [transtwo@github.com/juju/arrar/test_functions_test.go:24]\n" +
				"one",
		}, {
			err: transthree(),
			expected: "transthree: translated [transthree@github.com/juju/arrar/test_functions_test.go:28]\n" +
				"two: one [two@github.com/juju/arrar/test_functions_test.go:16]",
		}, {
			err: four(),
			expected: "four [four@github.com/juju/arrar/test_functions_test.go:32]\n" +
				"transthree: translated [transthree@github.com/juju/arrar/test_functions_test.go:28]\n" +
				"two: one [two@github.com/juju/arrar/test_functions_test.go:16]",
		}, {
			err:      method(),
			expected: "method [(*receiver).Func@github.com/juju/arrar/test_functions_test.go:42]",
		},
	} {
		stack := DetailedErrorStack(test.err, Default)
		c.Check(stack, gc.DeepEquals, test.expected)
	}
}
