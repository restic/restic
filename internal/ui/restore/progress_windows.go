package restore

import (
	"encoding/json"

	"github.com/restic/restic/internal/restic"
)

// incrementFilesFinished increments the files finished count if it is a main file
func (p *Progress) incrementFilesFinished(attrs map[restic.GenericAttributeType]json.RawMessage) {
	if string(attrs[restic.TypeIsADS]) != "true" {
		p.s.FilesFinished++
	}
}
