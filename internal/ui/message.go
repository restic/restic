package ui

import "github.com/restic/restic/internal/ui/termstatus"

// Message reports progress with messages of different verbosity.
type Message struct {
	term *termstatus.Terminal
	v    uint
}

// NewMessage returns a message progress reporter with underlying terminal
// term.
func NewMessage(term *termstatus.Terminal, verbosity uint) *Message {
	return &Message{
		term: term,
		v:    verbosity,
	}
}

// E reports an error
func (m *Message) E(msg string, args ...interface{}) {
	m.term.Errorf(msg, args...)
}

// P prints a message if verbosity >= 1, this is used for normal messages which
// are not errors.
func (m *Message) P(msg string, args ...interface{}) {
	if m.v >= 1 {
		m.term.Printf(msg, args...)
	}
}

// V prints a message if verbosity >= 2, this is used for verbose messages.
func (m *Message) V(msg string, args ...interface{}) {
	if m.v >= 2 {
		m.term.Printf(msg, args...)
	}
}

// VV prints a message if verbosity >= 3, this is used for debug messages.
func (m *Message) VV(msg string, args ...interface{}) {
	if m.v >= 3 {
		m.term.Printf(msg, args...)
	}
}
