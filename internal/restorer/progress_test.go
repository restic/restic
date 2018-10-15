package restorer

import (
	"testing"

	"github.com/restic/restic/internal/ui"
)

func TestTextTemplates(t *testing.T) {
	p := newProgressUI(ui.NewValidatingProgressUI())

	// these panic if text templates can't be parsed or executed
	p.startFileListing()
	p.startFileContent()
	p.startMetadata()
	p.startVerify()
}
