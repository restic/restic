package ui

import (
	"fmt"
	"io"
	"time"
)

type nilProgressUI struct {
}

var _ ProgressUI = &nilProgressUI{}

// NewNilProgressUI new ProgressUI instance that does not print any messages.
// Meant for use from tests
func NewNilProgressUI() ProgressUI {
	return &nilProgressUI{}
}

func (p *nilProgressUI) E(msg string, args ...interface{})  {}
func (p *nilProgressUI) P(msg string, args ...interface{})  {}
func (p *nilProgressUI) V(msg string, args ...interface{})  {}
func (p *nilProgressUI) VV(msg string, args ...interface{}) {}
func (p *nilProgressUI) StartPhase(progress func() string, status func() []string, percent func() (int64, int64), summary func(time.Duration)) {
}
func (p *nilProgressUI) Update(op func()) {}
func (p *nilProgressUI) FinishPhase()     {}

type simpleProgressUI struct {
	stdout    io.Writer
	stderr    io.Writer
	verbosity uint
}

var _ ProgressUI = &simpleProgressUI{}

// NewSimpleProgressUI returns new ProgressUI instances that prints E/P/V/VV messages to provided
// stdout/stderr streams but does not display any operation execution progress or summary.
func NewSimpleProgressUI(stdout io.Writer, stderr io.Writer, verbosity uint) ProgressUI {
	return &simpleProgressUI{
		stdout:    stdout,
		stderr:    stderr,
		verbosity: verbosity,
	}
}

func (p *simpleProgressUI) E(msg string, args ...interface{}) {
	fmt.Fprintf(p.stderr, msg, args...)
}
func (p *simpleProgressUI) P(msg string, args ...interface{}) {
	if p.verbosity > 0 {
		fmt.Fprintf(p.stdout, msg, args...)
	}
}
func (p *simpleProgressUI) V(msg string, args ...interface{}) {
	if p.verbosity > 1 {
		fmt.Fprintf(p.stdout, msg, args...)
	}
}
func (p *simpleProgressUI) VV(msg string, args ...interface{}) {
	if p.verbosity > 2 {
		fmt.Fprintf(p.stdout, msg, args...)
	}
}
func (p *simpleProgressUI) StartPhase(progress func() string, status func() []string, percent func() (int64, int64), summary func(time.Duration)) {
}
func (p *simpleProgressUI) Update(op func()) {}
func (p *simpleProgressUI) FinishPhase()     {}
