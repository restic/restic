package archiver

import "github.com/restic/restic/internal/data"

// ItemAction describes backup progress for a single file or directory.
// The zero value indicates an error or incomplete item.
type ItemAction string

// Constants for the different CompleteItem actions.
const (
	ActionDirNew        ItemAction = "dir new"
	ActionDirUnchanged  ItemAction = "dir unchanged"
	ActionDirModified   ItemAction = "dir modified"
	ActionFileNew       ItemAction = "file new"
	ActionFileUnchanged ItemAction = "file unchanged"
	ActionFileModified  ItemAction = "file modified"
)

// IsZero reports whether the action describes a zero value.
func (a ItemAction) IsZero() bool {
	return a == ""
}

func itemProgressAction(previous, current *data.Node) ItemAction {
	if current == nil {
		return ""
	}

	switch current.Type {
	case data.NodeTypeDir:
		switch {
		case previous == nil:
			return ActionDirNew
		case previous.Equals(*current):
			return ActionDirUnchanged
		default:
			return ActionDirModified
		}

	case data.NodeTypeFile:
		switch {
		case previous == nil:
			return ActionFileNew
		case previous.Equals(*current):
			return ActionFileUnchanged
		default:
			return ActionFileModified
		}

	default:
		return ""
	}
}
