package ui

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
