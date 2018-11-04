package ui

import "time"

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
func (p *validatingProgressUI) StartPhase(progress func() string, status func() []string, percent func() (int64, int64), summary func(time.Duration)) {
}
func (p *validatingProgressUI) Update(op func()) {}
func (p *validatingProgressUI) FinishPhase()     {}
