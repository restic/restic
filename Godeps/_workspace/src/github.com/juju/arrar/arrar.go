// arrar is a package that provides an easy way to annotate errors without
// losing the orginal error context.
package arrar

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

var prefixSize int

func init() {
	_, file, _, ok := runtime.Caller(0)
	if ok {
		// We know that the end of the file should be:
		// github.com/juju/arrar/arrar.go
		size := len(file)
		suffix := len("github.com/juju/arrar/arrar.go")
		prefixSize = len(file[:size-suffix])
	}
}

type annotation struct {
	message  string
	err      error
	function string
	file     string
	line     int
}

type annotatedError struct {
	stack []annotation
}

var _ error = (*annotatedError)(nil)

func errorString(annotations []annotation) string {
	parts := []string{}
	var err error
	var origin string
	size := len(annotations)
	for i, a := range annotations {
		err = a.err
		if a.message != "" {
			parts = append(parts, a.message)
		}
		if err != nil && i < size-1 {
			origin = fmt.Sprintf(" (%s)", errorString(annotations[i+1:]))
			break
		}
	}
	message := strings.Join(parts, ", ")
	if message == "" {
		return fmt.Sprintf("%v%s", err, origin)
	}
	return fmt.Sprintf("%s: %v%s", message, err, origin)
}

// Error returns the annotation where the annotations are joined by commas,
// and separated by the original error by a colon. If there are translated
// errors in the stack, the Error representation of the previous annotations
// are in brackets.
func (a *annotatedError) Error() string {
	return errorString(a.stack)
}

func (a *annotatedError) addAnnotation(an annotation) *annotatedError {
	a.stack = append(
		[]annotation{an},
		a.stack...)
	return a
}

// LastError returns the most recent error recorded in the stack.
func (a *annotatedError) LastError() error {
	for _, annotation := range a.stack {
		if annotation.err != nil {
			return annotation.err
		}
	}
	return nil
}

func partialPath(filename string, elements int) string {
	if filename == "" {
		return ""
	}
	if elements < 1 {
		return filename
	}
	base := filepath.Base(filename)
	if elements == 1 {
		return base
	}
	dir := filepath.Dir(filename)
	if dir != "." {
		dir = partialPath(dir, elements-1)
	}
	return filepath.Join(dir, base)
}

func newAnnotation(message string) annotation {
	result := annotation{message: message}
	pc, file, line, ok := runtime.Caller(3)
	if ok {
		// Strip off the user's filesystem prefix from the file
		// to just end up with the path from $GOPATH/src
		result.file = file[prefixSize:]
		result.line = line
		result.function = function(runtime.FuncForPC(pc).Name())
	}
	return result
}

func annotate(err error, message string) error {
	a := newAnnotation(message)
	if annotated, ok := err.(*annotatedError); ok {
		return annotated.addAnnotation(a)
	}
	a.err = err
	return &annotatedError{[]annotation{a}}
}

// New creates a new error and records the location that the error is created.
func New(format string, args ...interface{}) error {
	return annotate(errors.New(fmt.Sprintf(format, args...)), "")
}

// Trace records the location of the Trace call, and adds it to the annotation
// stack.
func Trace(err error) error {
	return annotate(err, "")
}

// Annotate is used to add extra context to an existing error. The location of
// the Annotate call is recorded with the annotations. The file, line and
// function are also recorded.
func Annotate(err error, message string) error {
	return annotate(err, message)
}

// Annotatef operates like Annotate, but uses the a format and args that match
// the fmt package.
func Annotatef(err error, format string, args ...interface{}) error {
	return annotate(err, fmt.Sprintf(format, args...))
}

func translate(err, newError error, message string) error {
	a := newAnnotation(message)
	a.err = newError
	if annotated, ok := err.(*annotatedError); ok {
		return annotated.addAnnotation(a)
	}
	return &annotatedError{
		[]annotation{
			a,
			{err: err},
		},
	}
}

// Translate records the newError along with the message in the annotation
// stack.
func Translate(err, newError error, message string) error {
	return translate(err, newError, message)
}

// Translatef operates like Translate, but uses the a format and args that
// match the fmt package.
func Translatef(err, newError error, format string, args ...interface{}) error {
	return translate(err, newError, fmt.Sprintf(format, args))
}

// Check looks at the underling error to see if it matches the checker
// function.
func Check(err error, checker func(error) bool) bool {
	if annotated, ok := err.(*annotatedError); ok {
		return checker(annotated.LastError())
	}
	return checker(err)
}

// GetErrorStack returns a slice of errors stored in the annotated errors. If
// the error isn't an annotated error, a slice with a single value is
// returned.
func GetErrorStack(err error) []error {
	if annotated, ok := err.(*annotatedError); ok {
		result := []error{}
		for _, a := range annotated.stack {
			if a.err != nil {
				result = append(result, a.err)
			}
		}
		return result
	}
	return []error{err}
}

// OutputParams are used to control the look of the DetailedErrorStack.
type OutputParams struct {
	Prefix    string
	FileDepth int
}

// Default is a simple pre-defined params for the DetailedErrorStack method
// that has no prefix, and shows files to a depth of one.
var Default = OutputParams{FileDepth: 0}

// DetailedErrorStack gives a slice containing the detailed error stack,
// annotation and original error, along with the location if it was recorded.
func DetailedErrorStack(err error, params OutputParams) string {
	if annotated, ok := err.(*annotatedError); ok {
		result := []string{}
		for _, a := range annotated.stack {
			errText := ""
			if a.err != nil {
				format := ": %v"
				if a.message == "" {
					format = "%v"
				}
				errText = fmt.Sprintf(format, a.err)
			}
			line := fmt.Sprintf(
				"%s%s%s%s",
				params.Prefix,
				a.message,
				errText,
				a.location(params.FileDepth))
			result = append(result, line)
		}
		return strings.Join(result, "\n")
	}
	return err.Error()
}

func (a *annotation) location(depth int) string {
	if a.file != "" {
		format := " [%s@%s:%v]"
		if a.function == "" {
			format = " [%s%s:%v]"
		}
		return fmt.Sprintf(format, a.function, partialPath(a.file, depth), a.line)
	}
	return ""
}
