package backup

import (
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
)

type mockPrinter struct {
	sync.Mutex
	dirUnchanged, fileNew bool
	id                    restic.ID
}

func (p *mockPrinter) Update(_, _ Counter, _ uint, _ map[string]struct{}, _ time.Time, _ uint64) {
}
func (p *mockPrinter) Error(_ string, err error) error        { return err }
func (p *mockPrinter) ScannerError(_ string, err error) error { return err }

func (p *mockPrinter) CompleteItem(messageType string, _ string, _ archiver.ItemStats, _ time.Duration) {
	p.Lock()
	defer p.Unlock()

	switch messageType {
	case "dir unchanged":
		p.dirUnchanged = true
	case "file new":
		p.fileNew = true
	}
}

func (p *mockPrinter) ReportTotal(_ time.Time, _ archiver.ScanStats) {}
func (p *mockPrinter) Finish(id restic.ID, _ time.Time, summary *Summary, _ bool) {
	p.Lock()
	defer p.Unlock()

	_ = *summary // Should not be nil.
	p.id = id
}

func (p *mockPrinter) Reset() {}

func (p *mockPrinter) P(_ string, _ ...interface{}) {}
func (p *mockPrinter) V(_ string, _ ...interface{}) {}

func TestProgress(t *testing.T) {
	t.Parallel()

	prnt := &mockPrinter{}
	prog := NewProgress(prnt, time.Millisecond)

	prog.StartFile("foo")
	prog.CompleteBlob(1024)

	// "dir unchanged"
	node := restic.Node{Type: "dir"}
	prog.CompleteItem("foo", &node, &node, archiver.ItemStats{}, 0)
	// "file new"
	node.Type = "file"
	prog.CompleteItem("foo", nil, &node, archiver.ItemStats{}, 0)

	time.Sleep(10 * time.Millisecond)
	id := restic.NewRandomID()
	prog.Finish(id, false)

	if !prnt.dirUnchanged {
		t.Error(`"dir unchanged" event not seen`)
	}
	if !prnt.fileNew {
		t.Error(`"file new" event not seen`)
	}
	if prnt.id != id {
		t.Errorf("id not stored (has %v)", prnt.id)
	}
}
