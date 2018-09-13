package ui

type testProgressUI struct {
}

var _ ProgressUI = &testProgressUI{}

// NewNilProgressUI new ProgressUI instance that does not print any messages.
// Meant for use from tests
func NewNilProgressUI() ProgressUI {
	return &testProgressUI{}
}

func (p *testProgressUI) E(msg string, args ...interface{})                   {}
func (p *testProgressUI) P(msg string, args ...interface{})                   {}
func (p *testProgressUI) V(msg string, args ...interface{})                   {}
func (p *testProgressUI) VV(msg string, args ...interface{})                  {}
func (p *testProgressUI) Set(title string, progress, summary func() []string) {}
func (p *testProgressUI) Update(op func())                                    {}
