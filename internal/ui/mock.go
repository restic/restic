package ui

import (
	"context"
	"io"
)

var _ Terminal = &MockTerminal{}

type MockTerminal struct {
	Output []string
	Errors []string
}

func (m *MockTerminal) Print(line string) {
	m.Output = append(m.Output, line)
}

func (m *MockTerminal) Error(line string) {
	m.Errors = append(m.Errors, line)
}

func (m *MockTerminal) SetStatus(lines []string) {
	m.Output = append([]string{}, lines...)
}

func (m *MockTerminal) CanUpdateStatus() bool {
	return true
}

func (m *MockTerminal) InputRaw() io.ReadCloser {
	return nil
}

func (m *MockTerminal) InputIsTerminal() bool {
	return true
}

func (m *MockTerminal) ReadPassword(_ context.Context, _ string) (string, error) {
	return "password", nil
}

func (m *MockTerminal) OutputWriter() io.Writer {
	return nil
}

func (m *MockTerminal) OutputRaw() io.Writer {
	return nil
}

func (m *MockTerminal) OutputIsTerminal() bool {
	return true
}
