package flags

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestWriteIni(t *testing.T) {
	oldEnv := EnvSnapshot()
	defer oldEnv.Restore()
	os.Setenv("ENV_DEFAULT", "env-def")

	var tests = []struct {
		args     []string
		options  IniOptions
		expected string
	}{
		{
			[]string{"-vv", "--intmap=a:2", "--intmap", "b:3", "filename", "0", "command"},
			IniDefault,
			`[Application Options]
; Show verbose debug information
verbose = true
verbose = true

; Test env-default1 value
EnvDefault1 = env-def

; Test env-default2 value
EnvDefault2 = env-def

[Other Options]
; A map from string to int
int-map = a:2
int-map = b:3

`,
		},
		{
			[]string{"-vv", "--intmap=a:2", "--intmap", "b:3", "filename", "0", "command"},
			IniDefault | IniIncludeDefaults,
			`[Application Options]
; Show verbose debug information
verbose = true
verbose = true

; A slice of pointers to string
; PtrSlice =

EmptyDescription = false

; Test default value
Default = "Some\nvalue"

; Test default array value
DefaultArray = Some value
DefaultArray = "Other\tvalue"

; Testdefault map value
DefaultMap = another:value
DefaultMap = some:value

; Test env-default1 value
EnvDefault1 = env-def

; Test env-default2 value
EnvDefault2 = env-def

; Option with named argument
OptionWithArgName =

; Option only available in ini
only-ini =

[Other Options]
; A slice of strings
StringSlice = some
StringSlice = value

; A map from string to int
int-map = a:2
int-map = b:3

[Subgroup]
; This is a subgroup option
Opt =

[Subsubgroup]
; This is a subsubgroup option
Opt =

[command]
; Use for extra verbosity
; ExtraVerbose =

`,
		},
		{
			[]string{"filename", "0", "command"},
			IniDefault | IniIncludeDefaults | IniCommentDefaults,
			`[Application Options]
; Show verbose debug information
; verbose =

; A slice of pointers to string
; PtrSlice =

; EmptyDescription = false

; Test default value
; Default = "Some\nvalue"

; Test default array value
; DefaultArray = Some value
; DefaultArray = "Other\tvalue"

; Testdefault map value
; DefaultMap = another:value
; DefaultMap = some:value

; Test env-default1 value
EnvDefault1 = env-def

; Test env-default2 value
EnvDefault2 = env-def

; Option with named argument
; OptionWithArgName =

; Option only available in ini
; only-ini =

[Other Options]
; A slice of strings
; StringSlice = some
; StringSlice = value

; A map from string to int
; int-map = a:1

[Subgroup]
; This is a subgroup option
; Opt =

[Subsubgroup]
; This is a subsubgroup option
; Opt =

[command]
; Use for extra verbosity
; ExtraVerbose =

`,
		},
		{
			[]string{"--default=New value", "--default-array=New value", "--default-map=new:value", "filename", "0", "command"},
			IniDefault | IniIncludeDefaults | IniCommentDefaults,
			`[Application Options]
; Show verbose debug information
; verbose =

; A slice of pointers to string
; PtrSlice =

; EmptyDescription = false

; Test default value
Default = New value

; Test default array value
DefaultArray = New value

; Testdefault map value
DefaultMap = new:value

; Test env-default1 value
EnvDefault1 = env-def

; Test env-default2 value
EnvDefault2 = env-def

; Option with named argument
; OptionWithArgName =

; Option only available in ini
; only-ini =

[Other Options]
; A slice of strings
; StringSlice = some
; StringSlice = value

; A map from string to int
; int-map = a:1

[Subgroup]
; This is a subgroup option
; Opt =

[Subsubgroup]
; This is a subsubgroup option
; Opt =

[command]
; Use for extra verbosity
; ExtraVerbose =

`,
		},
	}

	for _, test := range tests {
		var opts helpOptions

		p := NewNamedParser("TestIni", Default)
		p.AddGroup("Application Options", "The application options", &opts)

		_, err := p.ParseArgs(test.args)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		inip := NewIniParser(p)

		var b bytes.Buffer
		inip.Write(&b, test.options)

		got := b.String()
		expected := test.expected

		msg := fmt.Sprintf("with arguments %+v and ini options %b", test.args, test.options)
		assertDiff(t, got, expected, msg)
	}
}

func TestReadIni(t *testing.T) {
	var opts helpOptions

	p := NewNamedParser("TestIni", Default)
	p.AddGroup("Application Options", "The application options", &opts)

	inip := NewIniParser(p)

	inic := `
; Show verbose debug information
verbose = true
verbose = true

DefaultMap = another:"value\n1"
DefaultMap = some:value 2

[Application Options]
; A slice of pointers to string
; PtrSlice =

; Test default value
Default = "New\nvalue"

; Test env-default1 value
EnvDefault1 = New value

[Other Options]
# A slice of strings
StringSlice = "some\nvalue"
StringSlice = another value

; A map from string to int
int-map = a:2
int-map = b:3

`

	b := strings.NewReader(inic)
	err := inip.Parse(b)

	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	assertBoolArray(t, opts.Verbose, []bool{true, true})

	if v := map[string]string{"another": "value\n1", "some": "value 2"}; !reflect.DeepEqual(opts.DefaultMap, v) {
		t.Fatalf("Expected %#v for DefaultMap but got %#v", v, opts.DefaultMap)
	}

	assertString(t, opts.Default, "New\nvalue")

	assertString(t, opts.EnvDefault1, "New value")

	assertStringArray(t, opts.Other.StringSlice, []string{"some\nvalue", "another value"})

	if v, ok := opts.Other.IntMap["a"]; !ok {
		t.Errorf("Expected \"a\" in Other.IntMap")
	} else if v != 2 {
		t.Errorf("Expected Other.IntMap[\"a\"] = 2, but got %v", v)
	}

	if v, ok := opts.Other.IntMap["b"]; !ok {
		t.Errorf("Expected \"b\" in Other.IntMap")
	} else if v != 3 {
		t.Errorf("Expected Other.IntMap[\"b\"] = 3, but got %v", v)
	}
}

func TestReadAndWriteIni(t *testing.T) {
	var tests = []struct {
		options IniOptions
		read    string
		write   string
	}{
		{
			IniIncludeComments,
			`[Application Options]
; Show verbose debug information
verbose = true
verbose = true

; Test default value
Default = "quote me"

; Test default array value
DefaultArray = 1
DefaultArray = "2"
DefaultArray = 3

; Testdefault map value
; DefaultMap =

; Test env-default1 value
EnvDefault1 = env-def

; Test env-default2 value
EnvDefault2 = env-def

[Other Options]
; A slice of strings
; StringSlice =

; A map from string to int
int-map = a:2
int-map = b:"3"

`,
			`[Application Options]
; Show verbose debug information
verbose = true
verbose = true

; Test default value
Default = "quote me"

; Test default array value
DefaultArray = 1
DefaultArray = 2
DefaultArray = 3

; Testdefault map value
; DefaultMap =

; Test env-default1 value
EnvDefault1 = env-def

; Test env-default2 value
EnvDefault2 = env-def

[Other Options]
; A slice of strings
; StringSlice =

; A map from string to int
int-map = a:2
int-map = b:3

`,
		},
		{
			IniIncludeComments,
			`[Application Options]
; Show verbose debug information
verbose = true
verbose = true

; Test default value
Default = "quote me"

; Test default array value
DefaultArray = "1"
DefaultArray = "2"
DefaultArray = "3"

; Testdefault map value
; DefaultMap =

; Test env-default1 value
EnvDefault1 = env-def

; Test env-default2 value
EnvDefault2 = env-def

[Other Options]
; A slice of strings
; StringSlice =

; A map from string to int
int-map = a:"2"
int-map = b:"3"

`,
			`[Application Options]
; Show verbose debug information
verbose = true
verbose = true

; Test default value
Default = "quote me"

; Test default array value
DefaultArray = "1"
DefaultArray = "2"
DefaultArray = "3"

; Testdefault map value
; DefaultMap =

; Test env-default1 value
EnvDefault1 = env-def

; Test env-default2 value
EnvDefault2 = env-def

[Other Options]
; A slice of strings
; StringSlice =

; A map from string to int
int-map = a:"2"
int-map = b:"3"

`,
		},
	}

	for _, test := range tests {
		var opts helpOptions

		p := NewNamedParser("TestIni", Default)
		p.AddGroup("Application Options", "The application options", &opts)

		inip := NewIniParser(p)

		read := strings.NewReader(test.read)
		err := inip.Parse(read)
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}

		var write bytes.Buffer
		inip.Write(&write, test.options)

		got := write.String()

		msg := fmt.Sprintf("with ini options %b", test.options)
		assertDiff(t, got, test.write, msg)
	}
}

func TestReadIniWrongQuoting(t *testing.T) {
	var tests = []struct {
		iniFile    string
		lineNumber uint
	}{
		{
			iniFile:    `Default = "New\nvalue`,
			lineNumber: 1,
		},
		{
			iniFile:    `StringSlice = "New\nvalue`,
			lineNumber: 1,
		},
		{
			iniFile: `StringSlice = "New\nvalue"
			StringSlice = "Second\nvalue`,
			lineNumber: 2,
		},
		{
			iniFile:    `DefaultMap = some:"value`,
			lineNumber: 1,
		},
		{
			iniFile: `DefaultMap = some:value
			DefaultMap = another:"value`,
			lineNumber: 2,
		},
	}

	for _, test := range tests {
		var opts helpOptions

		p := NewNamedParser("TestIni", Default)
		p.AddGroup("Application Options", "The application options", &opts)

		inip := NewIniParser(p)

		inic := test.iniFile

		b := strings.NewReader(inic)
		err := inip.Parse(b)

		if err == nil {
			t.Fatalf("Expect error")
		}

		iniError := err.(*IniError)

		if iniError.LineNumber != test.lineNumber {
			t.Fatalf("Expect error on line %d", test.lineNumber)
		}
	}
}

func TestIniCommands(t *testing.T) {
	var opts struct {
		Value string `short:"v" long:"value"`

		Add struct {
			Name int `short:"n" long:"name" ini-name:"AliasName"`

			Other struct {
				O string `short:"o" long:"other"`
			} `group:"Other Options"`
		} `command:"add"`
	}

	p := NewNamedParser("TestIni", Default)
	p.AddGroup("Application Options", "The application options", &opts)

	inip := NewIniParser(p)

	inic := `[Application Options]
value = some value

[add]
AliasName = 5

[add.Other Options]
other = subgroup

`

	b := strings.NewReader(inic)
	err := inip.Parse(b)

	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	assertString(t, opts.Value, "some value")

	if opts.Add.Name != 5 {
		t.Errorf("Expected opts.Add.Name to be 5, but got %v", opts.Add.Name)
	}

	assertString(t, opts.Add.Other.O, "subgroup")

	// Test writing it back
	buf := &bytes.Buffer{}

	inip.Write(buf, IniDefault)

	assertDiff(t, buf.String(), inic, "ini contents")
}

func TestIniNoIni(t *testing.T) {
	var opts struct {
		NoValue string `short:"n" long:"novalue" no-ini:"yes"`
		Value   string `short:"v" long:"value"`
	}

	p := NewNamedParser("TestIni", Default)
	p.AddGroup("Application Options", "The application options", &opts)

	inip := NewIniParser(p)

	// read INI
	inic := `[Application Options]
novalue = some value
value = some other value
`

	b := strings.NewReader(inic)
	err := inip.Parse(b)

	if err == nil {
		t.Fatalf("Expected error")
	}

	iniError := err.(*IniError)

	if v := uint(2); iniError.LineNumber != v {
		t.Errorf("Expected opts.Add.Name to be %d, but got %d", v, iniError.LineNumber)
	}

	if v := "unknown option: novalue"; iniError.Message != v {
		t.Errorf("Expected opts.Add.Name to be %s, but got %s", v, iniError.Message)
	}

	// write INI
	opts.NoValue = "some value"
	opts.Value = "some other value"

	file, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("Cannot create temporary file: %s", err)
	}
	defer os.Remove(file.Name())

	err = inip.WriteFile(file.Name(), IniIncludeDefaults)
	if err != nil {
		t.Fatalf("Could not write ini file: %s", err)
	}

	found, err := ioutil.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("Could not read written ini file: %s", err)
	}

	expected := "[Application Options]\nValue = some other value\n\n"

	assertDiff(t, string(found), expected, "ini content")
}

func TestIniParse(t *testing.T) {
	file, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("Cannot create temporary file: %s", err)
	}
	defer os.Remove(file.Name())

	_, err = file.WriteString("value = 123")
	if err != nil {
		t.Fatalf("Cannot write to temporary file: %s", err)
	}

	file.Close()

	var opts struct {
		Value int `long:"value"`
	}

	err = IniParse(file.Name(), &opts)
	if err != nil {
		t.Fatalf("Could not parse ini: %s", err)
	}

	if opts.Value != 123 {
		t.Fatalf("Expected Value to be \"123\" but was \"%d\"", opts.Value)
	}
}

func TestWriteFile(t *testing.T) {
	file, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("Cannot create temporary file: %s", err)
	}
	defer os.Remove(file.Name())

	var opts struct {
		Value int `long:"value"`
	}

	opts.Value = 123

	p := NewParser(&opts, Default)
	ini := NewIniParser(p)

	err = ini.WriteFile(file.Name(), IniIncludeDefaults)
	if err != nil {
		t.Fatalf("Could not write ini file: %s", err)
	}

	found, err := ioutil.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("Could not read written ini file: %s", err)
	}

	expected := "[Application Options]\nValue = 123\n\n"

	assertDiff(t, string(found), expected, "ini content")
}

func TestOverwriteRequiredOptions(t *testing.T) {
	var tests = []struct {
		args     []string
		expected []string
	}{
		{
			args: []string{"--value", "from CLI"},
			expected: []string{
				"from CLI",
				"from default",
			},
		},
		{
			args: []string{"--value", "from CLI", "--default", "from CLI"},
			expected: []string{
				"from CLI",
				"from CLI",
			},
		},
		{
			args: []string{"--config", "no file name"},
			expected: []string{
				"from INI",
				"from INI",
			},
		},
		{
			args: []string{"--value", "from CLI before", "--default", "from CLI before", "--config", "no file name"},
			expected: []string{
				"from INI",
				"from INI",
			},
		},
		{
			args: []string{"--value", "from CLI before", "--default", "from CLI before", "--config", "no file name", "--value", "from CLI after", "--default", "from CLI after"},
			expected: []string{
				"from CLI after",
				"from CLI after",
			},
		},
	}

	for _, test := range tests {
		var opts struct {
			Config  func(s string) error `long:"config" no-ini:"true"`
			Value   string               `long:"value" required:"true"`
			Default string               `long:"default" required:"true" default:"from default"`
		}

		p := NewParser(&opts, Default)

		opts.Config = func(s string) error {
			ini := NewIniParser(p)

			return ini.Parse(bytes.NewBufferString("value = from INI\ndefault = from INI"))
		}

		_, err := p.ParseArgs(test.args)
		if err != nil {
			t.Fatalf("Unexpected error %s with args %+v", err, test.args)
		}

		if opts.Value != test.expected[0] {
			t.Fatalf("Expected Value to be \"%s\" but was \"%s\" with args %+v", test.expected[0], opts.Value, test.args)
		}

		if opts.Default != test.expected[1] {
			t.Fatalf("Expected Default to be \"%s\" but was \"%s\" with args %+v", test.expected[1], opts.Default, test.args)
		}
	}
}
