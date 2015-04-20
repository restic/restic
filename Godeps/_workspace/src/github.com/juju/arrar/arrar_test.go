package arrar

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"
)

func Test(t *testing.T) { gc.TestingT(t) }

type arrarSuite struct{}

var _ = gc.Suite(&arrarSuite{})

func echo(value interface{}) interface{} {
	return value
}

func (*arrarSuite) TestAnnotateOnUncomparableError(c *gc.C) {
	err := Annotate(newError("uncomparable"), "annotation")
	err = Annotate(err, "another")
	value := err.Error()
	c.Assert(value, gc.Equals, `another, annotation: uncomparable`)
	c.Assert(err, gc.ErrorMatches, `another, annotation: uncomparable`)
}

func (*arrarSuite) TestErrorStringOneAnnotation(c *gc.C) {
	first := fmt.Errorf("first error")
	err := Annotate(first, "annotation")
	c.Assert(err, gc.ErrorMatches, `annotation: first error`)
}

func (*arrarSuite) TestErrorStringTwoAnnotations(c *gc.C) {
	first := fmt.Errorf("first error")
	err := Annotate(first, "annotation")
	err = Annotate(err, "another")
	c.Assert(err, gc.ErrorMatches, `another, annotation: first error`)
}

func (*arrarSuite) TestErrorStringThreeAnnotations(c *gc.C) {
	first := fmt.Errorf("first error")
	err := Annotate(first, "annotation")
	err = Annotate(err, "another")
	err = Annotate(err, "third")
	c.Assert(err, gc.ErrorMatches, `third, another, annotation: first error`)
}

func (*arrarSuite) TestExampleAnnotations(c *gc.C) {
	for _, test := range []struct {
		err      error
		expected string
	}{
		{
			err:      two(),
			expected: "two: one",
		}, {
			err:      three(),
			expected: "three, two: one",
		}, {
			err:      transtwo(),
			expected: "transtwo: translated (one)",
		}, {
			err:      transthree(),
			expected: "transthree: translated (two: one)",
		}, {
			err:      four(),
			expected: "four, transthree: translated (two: one)",
		},
	} {
		c.Assert(test.err.Error(), gc.Equals, test.expected)
	}
}

func (*arrarSuite) TestAnnotatedErrorCheck(c *gc.C) {
	// Look for a file that we know isn't there.
	dir := c.MkDir()
	_, err := os.Stat(filepath.Join(dir, "not-there"))
	c.Assert(os.IsNotExist(err), gc.Equals, true)
	c.Assert(Check(err, os.IsNotExist), gc.Equals, true)

	err = Annotate(err, "wrap it")
	// Now the error itself isn't a 'IsNotExist'.
	c.Assert(os.IsNotExist(err), gc.Equals, false)
	// However if we use the Check method, it is.
	c.Assert(Check(err, os.IsNotExist), gc.Equals, true)
}

func (*arrarSuite) TestGetErrorStack(c *gc.C) {
	for _, test := range []struct {
		err     error
		matches []string
	}{
		{
			err:     fmt.Errorf("testing"),
			matches: []string{"testing"},
		}, {
			err:     Annotate(fmt.Errorf("testing"), "annotation"),
			matches: []string{"testing"},
		}, {
			err:     one(),
			matches: []string{"one"},
		}, {
			err:     two(),
			matches: []string{"one"},
		}, {
			err:     three(),
			matches: []string{"one"},
		}, {
			err:     transtwo(),
			matches: []string{"translated", "one"},
		}, {
			err:     transthree(),
			matches: []string{"translated", "one"},
		}, {
			err:     four(),
			matches: []string{"translated", "one"},
		},
	} {
		stack := GetErrorStack(test.err)
		c.Assert(stack, gc.HasLen, len(test.matches))
		for i, match := range test.matches {
			c.Assert(stack[i], gc.ErrorMatches, match)
		}
	}
}

// This is an uncomparable error type, as it is a struct that supports the
// error interface (as opposed to a pointer type).
type error_ struct {
	info  string
	slice []string
}

// Create a non-comparable error
func newError(message string) error {
	return error_{info: message}
}

func (e error_) Error() string {
	return e.info
}
