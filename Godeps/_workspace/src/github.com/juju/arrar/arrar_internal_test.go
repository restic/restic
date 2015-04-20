package arrar

import (
	"fmt"

	gc "launchpad.net/gocheck"
)

type internalSuite struct{}

var _ = gc.Suite(&internalSuite{})

func (*internalSuite) TestPartialPath(c *gc.C) {
	for i, test := range []struct {
		filename string
		count    int
		expected string
	}{
		{
			filename: "foo/bar/baz",
			count:    -1,
			expected: "foo/bar/baz",
		}, {
			filename: "foo/bar/baz",
			count:    0,
			expected: "foo/bar/baz",
		}, {
			filename: "foo/bar/baz",
			count:    1,
			expected: "baz",
		}, {
			filename: "foo/bar/baz",
			count:    2,
			expected: "bar/baz",
		}, {
			filename: "foo/bar/baz",
			count:    5,
			expected: "foo/bar/baz",
		}, {
			filename: "",
			count:    2,
			expected: "",
		},
	} {
		c.Logf("Test %v", i)
		c.Check(partialPath(test.filename, test.count), gc.Equals, test.expected)
	}
}

func (*internalSuite) TestInternalAnnotate(c *gc.C) {
	for i, test := range []struct {
		message  string
		errFunc  func() error
		expected []annotation
	}{
		{
			message: "structure of a simple annotation",
			errFunc: two,
			expected: []annotation{
				{
					message:  "two",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     16,
					function: "two",
					err:      fmt.Errorf("one"),
				},
			},
		}, {
			message: "structure of a stacked annotation",
			errFunc: three,
			expected: []annotation{
				{
					message:  "three",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     20,
					function: "three",
				}, {
					message:  "two",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     16,
					function: "two",
					err:      fmt.Errorf("one"),
				},
			},
		}, {
			message: "structure of a simple translation",
			errFunc: transtwo,
			expected: []annotation{
				{
					message:  "transtwo",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     24,
					function: "transtwo",
					err:      fmt.Errorf("translated"),
				}, {
					err: fmt.Errorf("one"),
				},
			},
		}, {
			message: "structure of a simple annotation then translated",
			errFunc: transthree,
			expected: []annotation{
				{
					message:  "transthree",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     28,
					function: "transthree",
					err:      fmt.Errorf("translated"),
				}, {
					message:  "two",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     16,
					function: "two",
					err:      fmt.Errorf("one"),
				},
			},
		}, {
			message: "structure of an annotationed, translated annotation",
			errFunc: four,
			expected: []annotation{
				{
					message:  "four",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     32,
					function: "four",
				}, {
					message:  "transthree",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     28,
					function: "transthree",
					err:      fmt.Errorf("translated"),
				}, {
					message:  "two",
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     16,
					function: "two",
					err:      fmt.Errorf("one"),
				},
			},
		}, {
			message: "test new",
			errFunc: test_new,
			expected: []annotation{
				{
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     36,
					function: "test_new",
					err:      fmt.Errorf("get location"),
				},
			},
		}, {
			message: "method",
			errFunc: method,
			expected: []annotation{
				{
					file:     "github.com/juju/arrar/test_functions_test.go",
					line:     42,
					function: "(*receiver).Func",
					err:      fmt.Errorf("method"),
				},
			},
		},
	} {
		c.Logf("%v: %s", i, test.message)
		err := test.errFunc()
		annotated, ok := err.(*annotatedError)
		c.Assert(ok, gc.Equals, true)
		c.Assert(annotated.stack, gc.HasLen, len(test.expected))
		for i, annotation := range test.expected {
			c.Logf("  %v: %v", i, annotation)
			stacked := annotated.stack[i]
			c.Check(stacked.message, gc.Equals, annotation.message)
			c.Check(stacked.file, gc.Equals, annotation.file)
			c.Check(stacked.function, gc.Equals, expectedFunctionResult(annotation.function))
			c.Check(stacked.line, gc.Equals, annotation.line)
			// We use ErrorMatches here as we can't easily test identity.
			if annotation.err == nil {
				c.Check(stacked.err, gc.IsNil)
			} else {
				c.Check(stacked.err, gc.ErrorMatches, fmt.Sprint(annotation.err))
			}
		}

	}
}
