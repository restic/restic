// These tests check that the foundations of gocheck are working properly.
// They already assume that fundamental failing is working already, though,
// since this was tested in bootstrap_test.go. Even then, some care may
// still have to be taken when using external functions, since they should
// of course not rely on functionality tested here.

package gocheck_test

import (
	"fmt"
	"launchpad.net/gocheck"
	"log"
	"os"
	"regexp"
	"strings"
)

// -----------------------------------------------------------------------
// Foundation test suite.

type FoundationS struct{}

var foundationS = gocheck.Suite(&FoundationS{})

func (s *FoundationS) TestCountSuite(c *gocheck.C) {
	suitesRun += 1
}

func (s *FoundationS) TestErrorf(c *gocheck.C) {
	// Do not use checkState() here.  It depends on Errorf() working.
	expectedLog := fmt.Sprintf("foundation_test.go:%d:\n"+
		"    c.Errorf(\"Error %%v!\", \"message\")\n"+
		"... Error: Error message!\n\n",
		getMyLine()+1)
	c.Errorf("Error %v!", "message")
	failed := c.Failed()
	c.Succeed()
	if log := c.GetTestLog(); log != expectedLog {
		c.Logf("Errorf() logged %#v rather than %#v", log, expectedLog)
		c.Fail()
	}
	if !failed {
		c.Logf("Errorf() didn't put the test in a failed state")
		c.Fail()
	}
}

func (s *FoundationS) TestError(c *gocheck.C) {
	expectedLog := fmt.Sprintf("foundation_test.go:%d:\n"+
		"    c\\.Error\\(\"Error \", \"message!\"\\)\n"+
		"\\.\\.\\. Error: Error message!\n\n",
		getMyLine()+1)
	c.Error("Error ", "message!")
	checkState(c, nil,
		&expectedState{
			name:   "Error(`Error `, `message!`)",
			failed: true,
			log:    expectedLog,
		})
}

func (s *FoundationS) TestFailNow(c *gocheck.C) {
	defer (func() {
		if !c.Failed() {
			c.Error("FailNow() didn't fail the test")
		} else {
			c.Succeed()
			if c.GetTestLog() != "" {
				c.Error("Something got logged:\n" + c.GetTestLog())
			}
		}
	})()

	c.FailNow()
	c.Log("FailNow() didn't stop the test")
}

func (s *FoundationS) TestSucceedNow(c *gocheck.C) {
	defer (func() {
		if c.Failed() {
			c.Error("SucceedNow() didn't succeed the test")
		}
		if c.GetTestLog() != "" {
			c.Error("Something got logged:\n" + c.GetTestLog())
		}
	})()

	c.Fail()
	c.SucceedNow()
	c.Log("SucceedNow() didn't stop the test")
}

func (s *FoundationS) TestFailureHeader(c *gocheck.C) {
	output := String{}
	failHelper := FailHelper{}
	gocheck.Run(&failHelper, &gocheck.RunConf{Output: &output})
	header := fmt.Sprintf(""+
		"\n-----------------------------------"+
		"-----------------------------------\n"+
		"FAIL: gocheck_test.go:%d: FailHelper.TestLogAndFail\n",
		failHelper.testLine)
	if strings.Index(output.value, header) == -1 {
		c.Errorf(""+
			"Failure didn't print a proper header.\n"+
			"... Got:\n%s... Expected something with:\n%s",
			output.value, header)
	}
}

func (s *FoundationS) TestFatal(c *gocheck.C) {
	var line int
	defer (func() {
		if !c.Failed() {
			c.Error("Fatal() didn't fail the test")
		} else {
			c.Succeed()
			expected := fmt.Sprintf("foundation_test.go:%d:\n"+
				"    c.Fatal(\"Die \", \"now!\")\n"+
				"... Error: Die now!\n\n",
				line)
			if c.GetTestLog() != expected {
				c.Error("Incorrect log:", c.GetTestLog())
			}
		}
	})()

	line = getMyLine() + 1
	c.Fatal("Die ", "now!")
	c.Log("Fatal() didn't stop the test")
}

func (s *FoundationS) TestFatalf(c *gocheck.C) {
	var line int
	defer (func() {
		if !c.Failed() {
			c.Error("Fatalf() didn't fail the test")
		} else {
			c.Succeed()
			expected := fmt.Sprintf("foundation_test.go:%d:\n"+
				"    c.Fatalf(\"Die %%s!\", \"now\")\n"+
				"... Error: Die now!\n\n",
				line)
			if c.GetTestLog() != expected {
				c.Error("Incorrect log:", c.GetTestLog())
			}
		}
	})()

	line = getMyLine() + 1
	c.Fatalf("Die %s!", "now")
	c.Log("Fatalf() didn't stop the test")
}

func (s *FoundationS) TestCallerLoggingInsideTest(c *gocheck.C) {
	log := fmt.Sprintf(""+
		"foundation_test.go:%d:\n"+
		"    result := c.Check\\(10, gocheck.Equals, 20\\)\n"+
		"\\.\\.\\. obtained int = 10\n"+
		"\\.\\.\\. expected int = 20\n\n",
		getMyLine()+1)
	result := c.Check(10, gocheck.Equals, 20)
	checkState(c, result,
		&expectedState{
			name:   "Check(10, Equals, 20)",
			result: false,
			failed: true,
			log:    log,
		})
}

func (s *FoundationS) TestCallerLoggingInDifferentFile(c *gocheck.C) {
	result, line := checkEqualWrapper(c, 10, 20)
	testLine := getMyLine() - 1
	log := fmt.Sprintf(""+
		"foundation_test.go:%d:\n"+
		"    result, line := checkEqualWrapper\\(c, 10, 20\\)\n"+
		"gocheck_test.go:%d:\n"+
		"    return c.Check\\(obtained, gocheck.Equals, expected\\), getMyLine\\(\\)\n"+
		"\\.\\.\\. obtained int = 10\n"+
		"\\.\\.\\. expected int = 20\n\n",
		testLine, line)
	checkState(c, result,
		&expectedState{
			name:   "Check(10, Equals, 20)",
			result: false,
			failed: true,
			log:    log,
		})
}

// -----------------------------------------------------------------------
// ExpectFailure() inverts the logic of failure.

type ExpectFailureSucceedHelper struct{}

func (s *ExpectFailureSucceedHelper) TestSucceed(c *gocheck.C) {
	c.ExpectFailure("It booms!")
	c.Error("Boom!")
}

type ExpectFailureFailHelper struct{}

func (s *ExpectFailureFailHelper) TestFail(c *gocheck.C) {
	c.ExpectFailure("Bug #XYZ")
}

func (s *FoundationS) TestExpectFailureFail(c *gocheck.C) {
	helper := ExpectFailureFailHelper{}
	output := String{}
	result := gocheck.Run(&helper, &gocheck.RunConf{Output: &output})

	expected := "" +
		"^\n-+\n" +
		"FAIL: foundation_test\\.go:[0-9]+:" +
		" ExpectFailureFailHelper\\.TestFail\n\n" +
		"\\.\\.\\. Error: Test succeeded, but was expected to fail\n" +
		"\\.\\.\\. Reason: Bug #XYZ\n$"

	matched, err := regexp.MatchString(expected, output.value)
	if err != nil {
		c.Error("Bad expression: ", expected)
	} else if !matched {
		c.Error("ExpectFailure() didn't log properly:\n", output.value)
	}

	c.Assert(result.ExpectedFailures, gocheck.Equals, 0)
}

func (s *FoundationS) TestExpectFailureSucceed(c *gocheck.C) {
	helper := ExpectFailureSucceedHelper{}
	output := String{}
	result := gocheck.Run(&helper, &gocheck.RunConf{Output: &output})

	c.Assert(output.value, gocheck.Equals, "")
	c.Assert(result.ExpectedFailures, gocheck.Equals, 1)
}

func (s *FoundationS) TestExpectFailureSucceedVerbose(c *gocheck.C) {
	helper := ExpectFailureSucceedHelper{}
	output := String{}
	result := gocheck.Run(&helper, &gocheck.RunConf{Output: &output, Verbose: true})

	expected := "" +
		"FAIL EXPECTED: foundation_test\\.go:[0-9]+:" +
		" ExpectFailureSucceedHelper\\.TestSucceed \\(It booms!\\)\t *[.0-9]+s\n"

	matched, err := regexp.MatchString(expected, output.value)
	if err != nil {
		c.Error("Bad expression: ", expected)
	} else if !matched {
		c.Error("ExpectFailure() didn't log properly:\n", output.value)
	}

	c.Assert(result.ExpectedFailures, gocheck.Equals, 1)
}

// -----------------------------------------------------------------------
// Skip() allows stopping a test without positive/negative results.

type SkipTestHelper struct{}

func (s *SkipTestHelper) TestFail(c *gocheck.C) {
	c.Skip("Wrong platform or whatever")
	c.Error("Boom!")
}

func (s *FoundationS) TestSkip(c *gocheck.C) {
	helper := SkipTestHelper{}
	output := String{}
	gocheck.Run(&helper, &gocheck.RunConf{Output: &output})

	if output.value != "" {
		c.Error("Skip() logged something:\n", output.value)
	}
}

func (s *FoundationS) TestSkipVerbose(c *gocheck.C) {
	helper := SkipTestHelper{}
	output := String{}
	gocheck.Run(&helper, &gocheck.RunConf{Output: &output, Verbose: true})

	expected := "SKIP: foundation_test\\.go:[0-9]+: SkipTestHelper\\.TestFail" +
		" \\(Wrong platform or whatever\\)"
	matched, err := regexp.MatchString(expected, output.value)
	if err != nil {
		c.Error("Bad expression: ", expected)
	} else if !matched {
		c.Error("Skip() didn't log properly:\n", output.value)
	}
}

// -----------------------------------------------------------------------
// Check minimum *log.Logger interface provided by *gocheck.C.

type minLogger interface {
	Output(calldepth int, s string) error
}

func (s *BootstrapS) TestMinLogger(c *gocheck.C) {
	var logger minLogger
	logger = log.New(os.Stderr, "", 0)
	logger = c
	logger.Output(0, "Hello there")
	expected := "\\[LOG\\] [.0-9]+ Hello there\n"
	output := c.GetTestLog()
	matched, err := regexp.MatchString(expected, output)
	if err != nil {
		c.Error("Bad expression: ", expected)
	} else if !matched {
		c.Error("Output() didn't log properly:\n", output)
	}
}

// -----------------------------------------------------------------------
// Ensure that suites with embedded types are working fine, including the
// the workaround for issue 906.

type EmbeddedInternalS struct {
	called bool
}

type EmbeddedS struct {
	EmbeddedInternalS
}

var embeddedS = gocheck.Suite(&EmbeddedS{})

func (s *EmbeddedS) TestCountSuite(c *gocheck.C) {
	suitesRun += 1
}

func (s *EmbeddedInternalS) TestMethod(c *gocheck.C) {
	c.Error("TestMethod() of the embedded type was called!?")
}

func (s *EmbeddedS) TestMethod(c *gocheck.C) {
	// http://code.google.com/p/go/issues/detail?id=906
	c.Check(s.called, gocheck.Equals, false) // Go issue 906 is affecting the runner?
	s.called = true
}
