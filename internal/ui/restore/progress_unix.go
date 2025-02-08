//go:build !windows
// +build !windows

package restore

import (
	"encoding/json"

	"github.com/restic/restic/internal/restic"
)

// incrementFilesFinished increments the files finished count
func (p *Progress) incrementFilesFinished(_ map[restic.GenericAttributeType]json.RawMessage) {
	p.s.FilesFinished++
}
