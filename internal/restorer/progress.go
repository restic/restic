package restorer

type ItemAction string

// Constants for the different CompleteItem actions.
const (
	ActionDirRestored   ItemAction = "dir restored"
	ActionFileRestored  ItemAction = "file restored"
	ActionFileUpdated   ItemAction = "file updated"
	ActionFileCloned    ItemAction = "file cloned"
	ActionFileUnchanged ItemAction = "file unchanged"
	ActionOtherRestored ItemAction = "other restored"
	ActionDeleted       ItemAction = "deleted"
)

// ProgressReporter reports restore progress.
type ProgressReporter interface {
	AddFile(size uint64)
	AddProgress(name string, action ItemAction, bytesWrittenPortion, bytesTotal uint64)
	AddClonedFile(name string, size uint64, blockCloned bool)
	AddSkippedFile(name string, size uint64)
	ReportDeletion(name string)
}

type noopProgressReporter struct{}

var _ ProgressReporter = (*noopProgressReporter)(nil)

func (noopProgressReporter) AddFile(uint64) {}
func (noopProgressReporter) AddProgress(string, ItemAction, uint64, uint64) {
}
func (noopProgressReporter) AddClonedFile(name string, size uint64, blockCloned bool) {}
func (noopProgressReporter) AddSkippedFile(string, uint64) {}
func (noopProgressReporter) ReportDeletion(string)         {}

func progressOrNoop(p ProgressReporter) ProgressReporter {
	if p == nil {
		return noopProgressReporter{}
	}
	return p
}
