package flags

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

type helpOptions struct {
	Verbose          []bool       `short:"v" long:"verbose" description:"Show verbose debug information" ini-name:"verbose"`
	Call             func(string) `short:"c" description:"Call phone number" ini-name:"call"`
	PtrSlice         []*string    `long:"ptrslice" description:"A slice of pointers to string"`
	EmptyDescription bool         `long:"empty-description"`

	Default           string            `long:"default" default:"Some\nvalue" description:"Test default value"`
	DefaultArray      []string          `long:"default-array" default:"Some value" default:"Other\tvalue" description:"Test default array value"`
	DefaultMap        map[string]string `long:"default-map" default:"some:value" default:"another:value" description:"Testdefault map value"`
	EnvDefault1       string            `long:"env-default1" default:"Some value" env:"ENV_DEFAULT" description:"Test env-default1 value"`
	EnvDefault2       string            `long:"env-default2" env:"ENV_DEFAULT" description:"Test env-default2 value"`
	OptionWithArgName string            `long:"opt-with-arg-name" value-name:"something" description:"Option with named argument"`

	OnlyIni string `ini-name:"only-ini" description:"Option only available in ini"`

	Other struct {
		StringSlice []string       `short:"s" default:"some" default:"value" description:"A slice of strings"`
		IntMap      map[string]int `long:"intmap" default:"a:1" description:"A map from string to int" ini-name:"int-map"`
	} `group:"Other Options"`

	Group struct {
		Opt string `long:"opt" description:"This is a subgroup option"`

		Group struct {
			Opt string `long:"opt" description:"This is a subsubgroup option"`
		} `group:"Subsubgroup" namespace:"sap"`
	} `group:"Subgroup" namespace:"sip"`

	Command struct {
		ExtraVerbose []bool `long:"extra-verbose" description:"Use for extra verbosity"`
	} `command:"command" alias:"cm" alias:"cmd" description:"A command"`

	Args struct {
		Filename string `positional-arg-name:"filename" description:"A filename"`
		Number   int    `positional-arg-name:"num" description:"A number"`
	} `positional-args:"yes"`
}

func TestHelp(t *testing.T) {
	oldEnv := EnvSnapshot()
	defer oldEnv.Restore()
	os.Setenv("ENV_DEFAULT", "env-def")

	var opts helpOptions
	p := NewNamedParser("TestHelp", HelpFlag)
	p.AddGroup("Application Options", "The application options", &opts)

	_, err := p.ParseArgs([]string{"--help"})

	if err == nil {
		t.Fatalf("Expected help error")
	}

	if e, ok := err.(*Error); !ok {
		t.Fatalf("Expected flags.Error, but got %T", err)
	} else {
		if e.Type != ErrHelp {
			t.Errorf("Expected flags.ErrHelp type, but got %s", e.Type)
		}

		var expected string

		if runtime.GOOS == "windows" {
			expected = `Usage:
  TestHelp [OPTIONS] [filename] [num] <command>

Application Options:
  /v, /verbose                         Show verbose debug information
  /c:                                  Call phone number
      /ptrslice:                       A slice of pointers to string
      /empty-description
      /default:                        Test default value ("Some\nvalue")
      /default-array:                  Test default array value (Some value, "Other\tvalue")
      /default-map:                    Testdefault map value (some:value, another:value)
      /env-default1:                   Test env-default1 value (Some value) [%ENV_DEFAULT%]
      /env-default2:                   Test env-default2 value [%ENV_DEFAULT%]
      /opt-with-arg-name:something     Option with named argument

Other Options:
  /s:                                  A slice of strings (some, value)
      /intmap:                         A map from string to int (a:1)

Subgroup:
      /sip.opt:                        This is a subgroup option

Subsubgroup:
      /sip.sap.opt:                    This is a subsubgroup option

Help Options:
  /?                                   Show this help message
  /h, /help                            Show this help message

Arguments:
  filename:                            A filename
  num:                                 A number

Available commands:
  command  A command (aliases: cm, cmd)
`
		} else {
			expected = `Usage:
  TestHelp [OPTIONS] [filename] [num] <command>

Application Options:
  -v, --verbose                        Show verbose debug information
  -c=                                  Call phone number
      --ptrslice=                      A slice of pointers to string
      --empty-description
      --default=                       Test default value ("Some\nvalue")
      --default-array=                 Test default array value (Some value,
                                       "Other\tvalue")
      --default-map=                   Testdefault map value (some:value,
                                       another:value)
      --env-default1=                  Test env-default1 value (Some value)
                                       [$ENV_DEFAULT]
      --env-default2=                  Test env-default2 value [$ENV_DEFAULT]
      --opt-with-arg-name=something    Option with named argument

Other Options:
  -s=                                  A slice of strings (some, value)
      --intmap=                        A map from string to int (a:1)

Subgroup:
      --sip.opt=                       This is a subgroup option

Subsubgroup:
      --sip.sap.opt=                   This is a subsubgroup option

Help Options:
  -h, --help                           Show this help message

Arguments:
  filename:                            A filename
  num:                                 A number

Available commands:
  command  A command (aliases: cm, cmd)
`
		}

		assertDiff(t, e.Message, expected, "help message")
	}
}

func TestMan(t *testing.T) {
	oldEnv := EnvSnapshot()
	defer oldEnv.Restore()
	os.Setenv("ENV_DEFAULT", "env-def")

	var opts helpOptions
	p := NewNamedParser("TestMan", HelpFlag)
	p.ShortDescription = "Test manpage generation"
	p.LongDescription = "This is a somewhat `longer' description of what this does"
	p.AddGroup("Application Options", "The application options", &opts)

	p.Commands()[0].LongDescription = "Longer `command' description"

	var buf bytes.Buffer
	p.WriteManPage(&buf)

	got := buf.String()

	tt := time.Now()

	expected := fmt.Sprintf(`.TH TestMan 1 "%s"
.SH NAME
TestMan \- Test manpage generation
.SH SYNOPSIS
\fBTestMan\fP [OPTIONS]
.SH DESCRIPTION
This is a somewhat \fBlonger\fP description of what this does
.SH OPTIONS
.TP
\fB-v, --verbose\fP
Show verbose debug information
.TP
\fB-c\fP
Call phone number
.TP
\fB--ptrslice\fP
A slice of pointers to string
.TP
\fB--empty-description\fP
.TP
\fB--default\fP
Test default value
.TP
\fB--default-array\fP
Test default array value
.TP
\fB--default-map\fP
Testdefault map value
.TP
\fB--env-default1\fP
Test env-default1 value
.TP
\fB--env-default2\fP
Test env-default2 value
.TP
\fB--opt-with-arg-name\fP
Option with named argument
.TP
\fB-s\fP
A slice of strings
.TP
\fB--intmap\fP
A map from string to int
.TP
\fB--sip.opt\fP
This is a subgroup option
.TP
\fB--sip.sap.opt\fP
This is a subsubgroup option
.SH COMMANDS
.SS command
A command

Longer \fBcommand\fP description

\fBUsage\fP: TestMan [OPTIONS] command [command-OPTIONS]


\fBAliases\fP: cm, cmd

.TP
\fB--extra-verbose\fP
Use for extra verbosity
`, tt.Format("2 January 2006"))

	assertDiff(t, got, expected, "man page")
}

type helpCommandNoOptions struct {
	Command struct {
	} `command:"command" description:"A command"`
}

func TestHelpCommand(t *testing.T) {
	oldEnv := EnvSnapshot()
	defer oldEnv.Restore()
	os.Setenv("ENV_DEFAULT", "env-def")

	var opts helpCommandNoOptions
	p := NewNamedParser("TestHelpCommand", HelpFlag)
	p.AddGroup("Application Options", "The application options", &opts)

	_, err := p.ParseArgs([]string{"command", "--help"})

	if err == nil {
		t.Fatalf("Expected help error")
	}

	if e, ok := err.(*Error); !ok {
		t.Fatalf("Expected flags.Error, but got %T", err)
	} else {
		if e.Type != ErrHelp {
			t.Errorf("Expected flags.ErrHelp type, but got %s", e.Type)
		}

		var expected string

		if runtime.GOOS == "windows" {
			expected = `Usage:
  TestHelpCommand [OPTIONS] command

Help Options:
  /?              Show this help message
  /h, /help       Show this help message
`
		} else {
			expected = `Usage:
  TestHelpCommand [OPTIONS] command

Help Options:
  -h, --help      Show this help message
`
		}

		assertDiff(t, e.Message, expected, "help message")
	}
}
