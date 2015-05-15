package gocheck

import (
	"fmt"
	"strings"
	"time"
)

// -----------------------------------------------------------------------
// Basic succeeding/failing logic.

// Return true if the currently running test has already failed.
func (c *C) Failed() bool {
	return c.status == failedSt
}

// Mark the currently running test as failed. Something ought to have been
// previously logged so that the developer knows what went wrong. The higher
// level helper functions will fail the test and do the logging properly.
func (c *C) Fail() {
	c.status = failedSt
}

// Mark the currently running test as failed, and stop running the test.
// Something ought to have been previously logged so that the developer
// knows what went wrong. The higher level helper functions will fail the
// test and do the logging properly.
func (c *C) FailNow() {
	c.Fail()
	c.stopNow()
}

// Mark the currently running test as succeeded, undoing any previous
// failures.
func (c *C) Succeed() {
	c.status = succeededSt
}

// Mark the currently running test as succeeded, undoing any previous
// failures, and stop running the test.
func (c *C) SucceedNow() {
	c.Succeed()
	c.stopNow()
}

// Expect the currently running test to fail, for the given reason.  If the
// test does not fail, an error will be reported to raise the attention to
// this fact. The reason string is just a summary of why the given test is
// supposed to fail.  This method is useful to temporarily disable tests
// which cover well known problems until a better time to fix the problem
// is found, without forgetting about the fact that a failure still exists.
func (c *C) ExpectFailure(reason string) {
	if reason == "" {
		panic("Missing reason why the test is expected to fail")
	}
	c.mustFail = true
	c.reason = reason
}

// Skip the running test, for the given reason.  If used within SetUpTest,
// the individual test being set up will be skipped, and in SetUpSuite it
// will cause the whole suite to be skipped.
func (c *C) Skip(reason string) {
	if reason == "" {
		panic("Missing reason why the test is being skipped")
	}
	c.reason = reason
	c.status = skippedSt
	c.stopNow()
}

// -----------------------------------------------------------------------
// Basic logging.

// Return the current test error output.
func (c *C) GetTestLog() string {
	return c.logb.String()
}

// Log some information into the test error output.  The provided arguments
// will be assembled together into a string using fmt.Sprint().
func (c *C) Log(args ...interface{}) {
	c.log(args...)
}

// Log some information into the test error output.  The provided arguments
// will be assembled together into a string using fmt.Sprintf().
func (c *C) Logf(format string, args ...interface{}) {
	c.logf(format, args...)
}

// Output enables *C to be used as a logger in functions that require only
// the minimum interface of *log.Logger.
func (c *C) Output(calldepth int, s string) error {
	ns := time.Now().Sub(time.Time{}).Nanoseconds()
	t := float64(ns%100e9) / 1e9
	c.Logf("[LOG] %.05f %s", t, s)
	return nil
}

// Log an error into the test error output, and mark the test as failed.
// The provided arguments will be assembled together into a string using
// fmt.Sprint().
func (c *C) Error(args ...interface{}) {
	c.logCaller(1)
	c.logString(fmt.Sprint("Error: ", fmt.Sprint(args...)))
	c.logNewLine()
	c.Fail()
}

// Log an error into the test error output, and mark the test as failed.
// The provided arguments will be assembled together into a string using
// fmt.Sprintf().
func (c *C) Errorf(format string, args ...interface{}) {
	c.logCaller(1)
	c.logString(fmt.Sprintf("Error: "+format, args...))
	c.logNewLine()
	c.Fail()
}

// Log an error into the test error output, mark the test as failed, and
// stop the test execution. The provided arguments will be assembled
// together into a string using fmt.Sprint().
func (c *C) Fatal(args ...interface{}) {
	c.logCaller(1)
	c.logString(fmt.Sprint("Error: ", fmt.Sprint(args...)))
	c.logNewLine()
	c.FailNow()
}

// Log an error into the test error output, mark the test as failed, and
// stop the test execution. The provided arguments will be assembled
// together into a string using fmt.Sprintf().
func (c *C) Fatalf(format string, args ...interface{}) {
	c.logCaller(1)
	c.logString(fmt.Sprint("Error: ", fmt.Sprintf(format, args...)))
	c.logNewLine()
	c.FailNow()
}

// -----------------------------------------------------------------------
// Generic checks and assertions based on checkers.

// Verify if the first value matches with the expected value.  What
// matching means is defined by the provided checker. In case they do not
// match, an error will be logged, the test will be marked as failed, and
// the test execution will continue.  Some checkers may not need the expected
// argument (e.g. IsNil).  In either case, any extra arguments provided to
// the function will be logged next to the reported problem when the
// matching fails.  This is a handy way to provide problem-specific hints.
func (c *C) Check(obtained interface{}, checker Checker, args ...interface{}) bool {
	return c.internalCheck("Check", obtained, checker, args...)
}

// Ensure that the first value matches with the expected value.  What
// matching means is defined by the provided checker. In case they do not
// match, an error will be logged, the test will be marked as failed, and
// the test execution will stop.  Some checkers may not need the expected
// argument (e.g. IsNil).  In either case, any extra arguments provided to
// the function will be logged next to the reported problem when the
// matching fails.  This is a handy way to provide problem-specific hints.
func (c *C) Assert(obtained interface{}, checker Checker, args ...interface{}) {
	if !c.internalCheck("Assert", obtained, checker, args...) {
		c.stopNow()
	}
}

func (c *C) internalCheck(funcName string, obtained interface{}, checker Checker, args ...interface{}) bool {
	if checker == nil {
		c.logCaller(2)
		c.logString(fmt.Sprintf("%s(obtained, nil!?, ...):", funcName))
		c.logString("Oops.. you've provided a nil checker!")
		c.logNewLine()
		c.Fail()
		return false
	}

	// If the last argument is a bug info, extract it out.
	var comment CommentInterface
	if len(args) > 0 {
		if c, ok := args[len(args)-1].(CommentInterface); ok {
			comment = c
			args = args[:len(args)-1]
		}
	}

	params := append([]interface{}{obtained}, args...)
	info := checker.Info()

	if len(params) != len(info.Params) {
		names := append([]string{info.Params[0], info.Name}, info.Params[1:]...)
		c.logCaller(2)
		c.logString(fmt.Sprintf("%s(%s):", funcName, strings.Join(names, ", ")))
		c.logString(fmt.Sprintf("Wrong number of parameters for %s: want %d, got %d", info.Name, len(names), len(params)+1))
		c.logNewLine()
		c.Fail()
		return false
	}

	// Copy since it may be mutated by Check.
	names := append([]string{}, info.Params...)

	// Do the actual check.
	result, error := checker.Check(params, names)
	if !result || error != "" {
		c.logCaller(2)
		for i := 0; i != len(params); i++ {
			c.logValue(names[i], params[i])
		}
		if comment != nil {
			c.logString(comment.CheckCommentString())
		}
		if error != "" {
			c.logString(error)
		}
		c.logNewLine()
		c.Fail()
		return false
	}
	return true
}
