package ui

import (
	"fmt"
)

// Message reports progress with messages of different verbosity.
type Message struct {
	term Terminal
	v    uint
}

// NewMessage returns a message progress reporter with underlying terminal
// term.
func NewMessage(term Terminal, verbosity uint) *Message {
	return &Message{
		term: term,
		v:    verbosity,
	}
}

// E reports an error
func (m *Message) E(msg string, args ...interface{}) {
	m.term.Error(fmt.Sprintf(msg, args...))
}

// P prints a message if verbosity >= 1, this is used for normal messages which
// are not errors.
func (m *Message) P(msg string, args ...interface{}) {
	if m.v >= 1 {
		m.term.Print(fmt.Sprintf(msg, args...))
	}
}

// V prints a message if verbosity >= 2, this is used for verbose messages.
func (m *Message) V(msg string, args ...interface{}) {
	if m.v >= 2 {
		m.term.Print(fmt.Sprintf(msg, args...))
	}
}

// VV prints a message if verbosity >= 3, this is used for debug messages.
func (m *Message) VV(msg string, args ...interface{}) {
	if m.v >= 3 {
		m.term.Print(fmt.Sprintf(msg, args...))
	}
}
