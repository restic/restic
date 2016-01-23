// +build ignore

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"text/template"
	"time"
)

var data struct {
	Package   string
	Funcs     []string
	Timestamp string
}

var testTemplate = `
// DO NOT EDIT!
// generated at {{ .Timestamp }}
package {{ .Package }}

import (
	"testing"

	"github.com/restic/restic/backend/test"
)

{{ range $f := .Funcs }}func Test{{ $f }}(t *testing.T){ test.{{ $f }}(t) }
{{ end }}
`

var testFile = flag.String("testfile", "../test/tests.go", "file to search test functions in")
var outputFile = flag.String("output", "backend_test.go", "output file to write generated code to")

func errx(err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

var funcRegex = regexp.MustCompile(`^func\s+([A-Z].*)\s*\(`)

func findTestFunctions() (funcs []string) {
	f, err := os.Open(*testFile)
	errx(err)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		match := funcRegex.FindStringSubmatch(sc.Text())
		if len(match) > 0 {
			funcs = append(funcs, match[1])
		}
	}

	if err := sc.Err(); err != nil {
		log.Fatalf("Error scanning file: %v", err)
	}

	errx(f.Close())
	return funcs
}

func generateOutput(wr io.Writer, data interface{}) {
	t := template.Must(template.New("backendtest").Parse(testTemplate))

	cmd := exec.Command("gofmt")
	cmd.Stdout = wr
	in, err := cmd.StdinPipe()
	errx(err)
	errx(cmd.Start())
	errx(t.Execute(in, data))
	errx(in.Close())
	errx(cmd.Wait())
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

	packageName := filepath.Base(dir)

	f, err := os.Create(*outputFile)
	errx(err)

	data.Package = packageName + "_test"
	data.Funcs = findTestFunctions()
	data.Timestamp = time.Now().Format("2006-02-01 15:04:05 -0700 MST")
	generateOutput(f, data)

	errx(f.Close())

	fmt.Printf("wrote backend tests for package %v\n", packageName)
}
