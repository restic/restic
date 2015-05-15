package gocheck_test

import (
	"errors"
	"launchpad.net/gocheck"
	"reflect"
	"runtime"
)

type CheckersS struct{}

var _ = gocheck.Suite(&CheckersS{})

func testInfo(c *gocheck.C, checker gocheck.Checker, name string, paramNames []string) {
	info := checker.Info()
	if info.Name != name {
		c.Fatalf("Got name %s, expected %s", info.Name, name)
	}
	if !reflect.DeepEqual(info.Params, paramNames) {
		c.Fatalf("Got param names %#v, expected %#v", info.Params, paramNames)
	}
}

func testCheck(c *gocheck.C, checker gocheck.Checker, result bool, error string, params ...interface{}) ([]interface{}, []string) {
	info := checker.Info()
	if len(params) != len(info.Params) {
		c.Fatalf("unexpected param count in test; expected %d got %d", len(info.Params), len(params))
	}
	names := append([]string{}, info.Params...)
	result_, error_ := checker.Check(params, names)
	if result_ != result || error_ != error {
		c.Fatalf("%s.Check(%#v) returned (%#v, %#v) rather than (%#v, %#v)",
			info.Name, params, result_, error_, result, error)
	}
	return params, names
}

func (s *CheckersS) TestComment(c *gocheck.C) {
	bug := gocheck.Commentf("a %d bc", 42)
	comment := bug.CheckCommentString()
	if comment != "a 42 bc" {
		c.Fatalf("Commentf returned %#v", comment)
	}
}

func (s *CheckersS) TestIsNil(c *gocheck.C) {
	testInfo(c, gocheck.IsNil, "IsNil", []string{"value"})

	testCheck(c, gocheck.IsNil, true, "", nil)
	testCheck(c, gocheck.IsNil, false, "", "a")

	testCheck(c, gocheck.IsNil, true, "", (chan int)(nil))
	testCheck(c, gocheck.IsNil, false, "", make(chan int))
	testCheck(c, gocheck.IsNil, true, "", (error)(nil))
	testCheck(c, gocheck.IsNil, false, "", errors.New(""))
	testCheck(c, gocheck.IsNil, true, "", ([]int)(nil))
	testCheck(c, gocheck.IsNil, false, "", make([]int, 1))
	testCheck(c, gocheck.IsNil, false, "", int(0))
}

func (s *CheckersS) TestNotNil(c *gocheck.C) {
	testInfo(c, gocheck.NotNil, "NotNil", []string{"value"})

	testCheck(c, gocheck.NotNil, false, "", nil)
	testCheck(c, gocheck.NotNil, true, "", "a")

	testCheck(c, gocheck.NotNil, false, "", (chan int)(nil))
	testCheck(c, gocheck.NotNil, true, "", make(chan int))
	testCheck(c, gocheck.NotNil, false, "", (error)(nil))
	testCheck(c, gocheck.NotNil, true, "", errors.New(""))
	testCheck(c, gocheck.NotNil, false, "", ([]int)(nil))
	testCheck(c, gocheck.NotNil, true, "", make([]int, 1))
}

func (s *CheckersS) TestNot(c *gocheck.C) {
	testInfo(c, gocheck.Not(gocheck.IsNil), "Not(IsNil)", []string{"value"})

	testCheck(c, gocheck.Not(gocheck.IsNil), false, "", nil)
	testCheck(c, gocheck.Not(gocheck.IsNil), true, "", "a")
}

type simpleStruct struct {
	i int
}

func (s *CheckersS) TestEquals(c *gocheck.C) {
	testInfo(c, gocheck.Equals, "Equals", []string{"obtained", "expected"})

	// The simplest.
	testCheck(c, gocheck.Equals, true, "", 42, 42)
	testCheck(c, gocheck.Equals, false, "", 42, 43)

	// Different native types.
	testCheck(c, gocheck.Equals, false, "", int32(42), int64(42))

	// With nil.
	testCheck(c, gocheck.Equals, false, "", 42, nil)

	// Slices
	testCheck(c, gocheck.Equals, false, "runtime error: comparing uncomparable type []uint8", []byte{1, 2}, []byte{1, 2})

	// Struct values
	testCheck(c, gocheck.Equals, true, "", simpleStruct{1}, simpleStruct{1})
	testCheck(c, gocheck.Equals, false, "", simpleStruct{1}, simpleStruct{2})

	// Struct pointers
	testCheck(c, gocheck.Equals, false, "", &simpleStruct{1}, &simpleStruct{1})
	testCheck(c, gocheck.Equals, false, "", &simpleStruct{1}, &simpleStruct{2})
}

func (s *CheckersS) TestDeepEquals(c *gocheck.C) {
	testInfo(c, gocheck.DeepEquals, "DeepEquals", []string{"obtained", "expected"})

	// The simplest.
	testCheck(c, gocheck.DeepEquals, true, "", 42, 42)
	testCheck(c, gocheck.DeepEquals, false, "", 42, 43)

	// Different native types.
	testCheck(c, gocheck.DeepEquals, false, "", int32(42), int64(42))

	// With nil.
	testCheck(c, gocheck.DeepEquals, false, "", 42, nil)

	// Slices
	testCheck(c, gocheck.DeepEquals, true, "", []byte{1, 2}, []byte{1, 2})
	testCheck(c, gocheck.DeepEquals, false, "", []byte{1, 2}, []byte{1, 3})

	// Struct values
	testCheck(c, gocheck.DeepEquals, true, "", simpleStruct{1}, simpleStruct{1})
	testCheck(c, gocheck.DeepEquals, false, "", simpleStruct{1}, simpleStruct{2})

	// Struct pointers
	testCheck(c, gocheck.DeepEquals, true, "", &simpleStruct{1}, &simpleStruct{1})
	testCheck(c, gocheck.DeepEquals, false, "", &simpleStruct{1}, &simpleStruct{2})
}

func (s *CheckersS) TestHasLen(c *gocheck.C) {
	testInfo(c, gocheck.HasLen, "HasLen", []string{"obtained", "n"})

	testCheck(c, gocheck.HasLen, true, "", "abcd", 4)
	testCheck(c, gocheck.HasLen, true, "", []int{1, 2}, 2)
	testCheck(c, gocheck.HasLen, false, "", []int{1, 2}, 3)

	testCheck(c, gocheck.HasLen, false, "n must be an int", []int{1, 2}, "2")
	testCheck(c, gocheck.HasLen, false, "obtained value type has no length", nil, 2)
}

func (s *CheckersS) TestErrorMatches(c *gocheck.C) {
	testInfo(c, gocheck.ErrorMatches, "ErrorMatches", []string{"value", "regex"})

	testCheck(c, gocheck.ErrorMatches, false, "Error value is nil", nil, "some error")
	testCheck(c, gocheck.ErrorMatches, false, "Value is not an error", 1, "some error")
	testCheck(c, gocheck.ErrorMatches, true, "", errors.New("some error"), "some error")
	testCheck(c, gocheck.ErrorMatches, true, "", errors.New("some error"), "so.*or")

	// Verify params mutation
	params, names := testCheck(c, gocheck.ErrorMatches, false, "", errors.New("some error"), "other error")
	c.Assert(params[0], gocheck.Equals, "some error")
	c.Assert(names[0], gocheck.Equals, "error")
}

func (s *CheckersS) TestMatches(c *gocheck.C) {
	testInfo(c, gocheck.Matches, "Matches", []string{"value", "regex"})

	// Simple matching
	testCheck(c, gocheck.Matches, true, "", "abc", "abc")
	testCheck(c, gocheck.Matches, true, "", "abc", "a.c")

	// Must match fully
	testCheck(c, gocheck.Matches, false, "", "abc", "ab")
	testCheck(c, gocheck.Matches, false, "", "abc", "bc")

	// String()-enabled values accepted
	testCheck(c, gocheck.Matches, true, "", reflect.ValueOf("abc"), "a.c")
	testCheck(c, gocheck.Matches, false, "", reflect.ValueOf("abc"), "a.d")

	// Some error conditions.
	testCheck(c, gocheck.Matches, false, "Obtained value is not a string and has no .String()", 1, "a.c")
	testCheck(c, gocheck.Matches, false, "Can't compile regex: error parsing regexp: missing closing ]: `[c$`", "abc", "a[c")
}

func (s *CheckersS) TestPanics(c *gocheck.C) {
	testInfo(c, gocheck.Panics, "Panics", []string{"function", "expected"})

	// Some errors.
	testCheck(c, gocheck.Panics, false, "Function has not panicked", func() bool { return false }, "BOOM")
	testCheck(c, gocheck.Panics, false, "Function must take zero arguments", 1, "BOOM")

	// Plain strings.
	testCheck(c, gocheck.Panics, true, "", func() { panic("BOOM") }, "BOOM")
	testCheck(c, gocheck.Panics, false, "", func() { panic("KABOOM") }, "BOOM")
	testCheck(c, gocheck.Panics, true, "", func() bool { panic("BOOM") }, "BOOM")

	// Error values.
	testCheck(c, gocheck.Panics, true, "", func() { panic(errors.New("BOOM")) }, errors.New("BOOM"))
	testCheck(c, gocheck.Panics, false, "", func() { panic(errors.New("KABOOM")) }, errors.New("BOOM"))

	type deep struct{ i int }
	// Deep value
	testCheck(c, gocheck.Panics, true, "", func() { panic(&deep{99}) }, &deep{99})

	// Verify params/names mutation
	params, names := testCheck(c, gocheck.Panics, false, "", func() { panic(errors.New("KABOOM")) }, errors.New("BOOM"))
	c.Assert(params[0], gocheck.ErrorMatches, "KABOOM")
	c.Assert(names[0], gocheck.Equals, "panic")

	// Verify a nil panic
	testCheck(c, gocheck.Panics, true, "", func() { panic(nil) }, nil)
	testCheck(c, gocheck.Panics, false, "", func() { panic(nil) }, "NOPE")
}

func (s *CheckersS) TestPanicMatches(c *gocheck.C) {
	testInfo(c, gocheck.PanicMatches, "PanicMatches", []string{"function", "expected"})

	// Error matching.
	testCheck(c, gocheck.PanicMatches, true, "", func() { panic(errors.New("BOOM")) }, "BO.M")
	testCheck(c, gocheck.PanicMatches, false, "", func() { panic(errors.New("KABOOM")) }, "BO.M")

	// Some errors.
	testCheck(c, gocheck.PanicMatches, false, "Function has not panicked", func() bool { return false }, "BOOM")
	testCheck(c, gocheck.PanicMatches, false, "Function must take zero arguments", 1, "BOOM")

	// Plain strings.
	testCheck(c, gocheck.PanicMatches, true, "", func() { panic("BOOM") }, "BO.M")
	testCheck(c, gocheck.PanicMatches, false, "", func() { panic("KABOOM") }, "BOOM")
	testCheck(c, gocheck.PanicMatches, true, "", func() bool { panic("BOOM") }, "BO.M")

	// Verify params/names mutation
	params, names := testCheck(c, gocheck.PanicMatches, false, "", func() { panic(errors.New("KABOOM")) }, "BOOM")
	c.Assert(params[0], gocheck.Equals, "KABOOM")
	c.Assert(names[0], gocheck.Equals, "panic")

	// Verify a nil panic
	testCheck(c, gocheck.PanicMatches, false, "Panic value is not a string or an error", func() { panic(nil) }, "")
}

func (s *CheckersS) TestFitsTypeOf(c *gocheck.C) {
	testInfo(c, gocheck.FitsTypeOf, "FitsTypeOf", []string{"obtained", "sample"})

	// Basic types
	testCheck(c, gocheck.FitsTypeOf, true, "", 1, 0)
	testCheck(c, gocheck.FitsTypeOf, false, "", 1, int64(0))

	// Aliases
	testCheck(c, gocheck.FitsTypeOf, false, "", 1, errors.New(""))
	testCheck(c, gocheck.FitsTypeOf, false, "", "error", errors.New(""))
	testCheck(c, gocheck.FitsTypeOf, true, "", errors.New("error"), errors.New(""))

	// Structures
	testCheck(c, gocheck.FitsTypeOf, false, "", 1, simpleStruct{})
	testCheck(c, gocheck.FitsTypeOf, false, "", simpleStruct{42}, &simpleStruct{})
	testCheck(c, gocheck.FitsTypeOf, true, "", simpleStruct{42}, simpleStruct{})
	testCheck(c, gocheck.FitsTypeOf, true, "", &simpleStruct{42}, &simpleStruct{})

	// Some bad values
	testCheck(c, gocheck.FitsTypeOf, false, "Invalid sample value", 1, interface{}(nil))
	testCheck(c, gocheck.FitsTypeOf, false, "", interface{}(nil), 0)
}

func (s *CheckersS) TestImplements(c *gocheck.C) {
	testInfo(c, gocheck.Implements, "Implements", []string{"obtained", "ifaceptr"})

	var e error
	var re runtime.Error
	testCheck(c, gocheck.Implements, true, "", errors.New(""), &e)
	testCheck(c, gocheck.Implements, false, "", errors.New(""), &re)

	// Some bad values
	testCheck(c, gocheck.Implements, false, "ifaceptr should be a pointer to an interface variable", 0, errors.New(""))
	testCheck(c, gocheck.Implements, false, "ifaceptr should be a pointer to an interface variable", 0, interface{}(nil))
	testCheck(c, gocheck.Implements, false, "", interface{}(nil), &e)
}
