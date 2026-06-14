package restorer

type progressInfoEntry struct {
	bytesWritten uint64
	bytesTotal   uint64
}

// progressState mirrors the progress counters asserted in restorer tests.
type progressState struct {
	FilesFinished   uint64
	FilesTotal      uint64
	FilesSkipped    uint64
	FilesDeleted    uint64
	AllBytesWritten uint64
	AllBytesTotal   uint64
	AllBytesSkipped uint64
}

// testProgress implements ProgressReporter for restorer tests.
type testProgress struct {
	progressInfoMap map[string]progressInfoEntry
	s               progressState
}

func newTestProgress() *testProgress {
	return &testProgress{
		progressInfoMap: make(map[string]progressInfoEntry),
	}
}

func (p *testProgress) AddFile(size uint64) {
	p.s.FilesTotal++
	p.s.AllBytesTotal += size
}

func (p *testProgress) AddProgress(name string, _ ItemAction, bytesWrittenPortion, bytesTotal uint64) {
	entry, exists := p.progressInfoMap[name]
	if !exists {
		entry.bytesTotal = bytesTotal
	}
	entry.bytesWritten += bytesWrittenPortion
	p.progressInfoMap[name] = entry

	p.s.AllBytesWritten += bytesWrittenPortion
	if entry.bytesWritten == entry.bytesTotal {
		delete(p.progressInfoMap, name)
		p.s.FilesFinished++
	}
}

func (p *testProgress) AddSkippedFile(_ string, size uint64) {
	p.s.FilesSkipped++
	p.s.AllBytesSkipped += size
}

func (p *testProgress) ReportDeletion(_ string) {
	p.s.FilesDeleted++
}

func (p *testProgress) state() progressState {
	return p.s
}
