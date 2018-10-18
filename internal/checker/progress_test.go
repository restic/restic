package checker

import (
	"testing"

	"github.com/restic/restic/internal/ui"
)

func TestIndexLoadStats(t *testing.T) {
	startIndexCheckProgress(ui.NewValidatingProgressUI())
}

func TestPackCheckStats(t *testing.T) {
	p := newPackCheckProgress(ui.NewValidatingProgressUI())
	p.startListPacks()
}

func TestStructureCheckStats(t *testing.T) {
	p := newStructureCheckStats(ui.NewValidatingProgressUI())
	p.startLoadSnapshots()
	p.startCheckSnapshots()
}

func TestReadPacksStats(t *testing.T) {
	startReadPacksProgress(ui.NewValidatingProgressUI(), 10)
}
