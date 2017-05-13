// +build ignore

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"
)

var data struct {
	Package        string
	TestFuncs      []string
	BenchmarkFuncs []string
}

var testTemplate = `
// DO NOT EDIT, AUTOMATICALLY GENERATED

package {{ .Package }}

import (
	"testing"
)

var testFunctions = []struct {
	Name string
	Fn   func(testing.TB, *Suite)
}{
{{ range $f := .TestFuncs -}}
	{"{{ $f }}", BackendTest{{ $f }},},
{{ end }}
}

var benchmarkFunctions = []struct {
	Name string
	Fn   func(*testing.B, *Suite)
}{
{{ range $f := .BenchmarkFuncs -}}
	{"{{ $f }}", BackendBenchmark{{ $f }},},
{{ end }}
}
`

var testFiles = flag.String("testfiles", "tests.go,benchmarks.go", "files to search test functions in (comma separated)")
var outputFile = flag.String("output", "funcs.go", "output file to write generated code to")
var packageName = flag.String("package", "", "the package name to use")
var prefix = flag.String("prefix", "", "test function prefix")
var quiet = flag.Bool("quiet", false, "be quiet")

func errx(err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

var testFuncRegex = regexp.MustCompile(`^func\s+BackendTest(.+)\s*\(`)
var benchmarkFuncRegex = regexp.MustCompile(`^func\s+BackendBenchmark(.+)\s*\(`)

func findFunctions() (testFuncs, benchmarkFuncs []string) {
	for _, filename := range strings.Split(*testFiles, ",") {
		f, err := os.Open(filename)
		errx(err)

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			match := testFuncRegex.FindStringSubmatch(sc.Text())
			if len(match) > 0 {
				testFuncs = append(testFuncs, match[1])
			}

			match = benchmarkFuncRegex.FindStringSubmatch(sc.Text())
			if len(match) > 0 {
				benchmarkFuncs = append(benchmarkFuncs, match[1])
			}
		}

		if err := sc.Err(); err != nil {
			log.Fatalf("Error scanning file: %v", err)
		}

		errx(f.Close())
	}

	return testFuncs, benchmarkFuncs
}

func generateOutput(wr io.Writer, data interface{}) {
	t := template.Must(template.New("backendtest").Parse(testTemplate))

	buf := bytes.NewBuffer(nil)
	errx(t.Execute(buf, data))

	source, err := format.Source(buf.Bytes())
	errx(err)

	_, err = wr.Write(source)
	errx(err)
}

func packageTestFunctionPrefix(pkg string) string {
	if pkg == "" {
		return ""
	}

	r, n := utf8.DecodeRuneInString(pkg)
	return string(unicode.ToUpper(r)) + pkg[n:]
}

func init() {
	flag.Parse()
}

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Getwd() %v\n", err)
		os.Exit(1)
	}

	pkg := *packageName
	if pkg == "" {
		pkg = filepath.Base(dir)
	}

	f, err := os.Create(*outputFile)
	errx(err)

	data.Package = pkg
	data.TestFuncs, data.BenchmarkFuncs = findFunctions()
	generateOutput(f, data)

	errx(f.Close())

	if !*quiet {
		fmt.Printf("wrote backend tests for package %v to %v\n", data.Package, *outputFile)
	}
}
