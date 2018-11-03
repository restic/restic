package ui

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
func (p *nilProgressUI) Set(title string, setup func(), metrics map[string]interface{}, progress string, status func() []string, summary string) {
}
func (p *nilProgressUI) Update(op func()) {}
func (p *nilProgressUI) Unset()           {}

type validatingProgressUI struct {
}

var _ ProgressUI = &validatingProgressUI{}

func NewValidatingProgressUI() ProgressUI {
	return &validatingProgressUI{}
}

func (p *validatingProgressUI) E(msg string, args ...interface{})  {}
func (p *validatingProgressUI) P(msg string, args ...interface{})  {}
func (p *validatingProgressUI) V(msg string, args ...interface{})  {}
func (p *validatingProgressUI) VV(msg string, args ...interface{}) {}
func (p *validatingProgressUI) Set(title string, setup func(), metrics map[string]interface{}, progress string, status func() []string, summary string) {
	executeTemplate(parseTemplate("progress", progress), metrics)
	executeTemplate(parseTemplate("summary", summary), metrics)
}
func (p *validatingProgressUI) Update(op func()) {}
func (p *validatingProgressUI) Unset()           {}
