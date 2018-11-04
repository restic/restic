package checker

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/ui"
)

type indexLoadStats struct {
	ui ui.ProgressUI

	totalIdxFiles, totalBlobs int64
}

func startIndexCheckProgress(ui ui.ProgressUI) *indexLoadStats {
	p := &indexLoadStats{ui: ui}

	progress := func() string {
		return fmt.Sprintf("Loading index files: %d index files (%d blobs)",
			p.totalIdxFiles,
			p.totalBlobs,
		)
	}
	summary := func(time.Duration) {
		p.ui.P("Loaded %d index files (%d blobs)",
			p.totalIdxFiles,
			p.totalBlobs,
		)
	}

	ui.StartPhase(progress, nil, nil, summary)

	return p
}

func (p *indexLoadStats) addIndexFile() {
	p.ui.Update(func() { p.totalIdxFiles++ })
}

func (p *indexLoadStats) doneIndexFile(blobCnt int) {
	p.ui.Update(func() { p.totalBlobs += int64(blobCnt) })
}

func (p *indexLoadStats) finish() {
	p.ui.FinishPhase()
}

type packCheckStats struct {
	ui ui.ProgressUI

	totalPackFiles int64
}

func newPackCheckProgress(ui ui.ProgressUI) *packCheckStats {
	return &packCheckStats{ui: ui}
}

func (p *packCheckStats) startListPacks() {
	progress := func() string {
		// "{{.packfiles.Value}} pack files"
		return fmt.Sprintf("Checking all packs: %d pack files", p.totalPackFiles)
	}
	summary := func(time.Duration) {
		p.ui.P("Checked %d pack files", p.totalPackFiles)
	}

	p.ui.StartPhase(progress, nil, nil, summary)
}

func (p *packCheckStats) addPack(ui ui.ProgressUI) {
	ui.Update(func() { p.totalPackFiles++ })
}

func (p *packCheckStats) finish() {
	p.ui.FinishPhase()
}

type structureCheckStats struct {
	ui ui.ProgressUI

	snapshotFiles ui.CounterTo
}

func newStructureCheckStats(ui ui.ProgressUI) *structureCheckStats {
	return &structureCheckStats{ui: ui}
}

func (p *structureCheckStats) startLoadSnapshots() {
	progress := func() string {
		return fmt.Sprintf("Loading snapshot files: %d snapshot files",
			p.snapshotFiles.Total,
		)
	}
	summary := func(time.Duration) {
		p.ui.P("Loaded %d snapshot files",
			p.snapshotFiles.Total,
		)
	}

	p.ui.StartPhase(progress, nil, nil, summary)
}

func (p *structureCheckStats) addSnapshot() {
	p.ui.Update(func() { p.snapshotFiles.Total++ })
}

func (p *structureCheckStats) startCheckSnapshots() {
	progress := func() string {
		return fmt.Sprintf("Checking snapshots, trees and blobs: %d snapshots",
			p.snapshotFiles.Current,
		)
	}
	summary := func(time.Duration) {
		p.ui.P("Checked %d snapshots",
			p.snapshotFiles.Current,
		)
	}

	p.ui.StartPhase(progress, nil, nil, summary)
}

func (p *structureCheckStats) doneSnapshot() {
	p.ui.Update(func() { p.snapshotFiles.Current++ })
}

func (p *structureCheckStats) finish() {
	p.ui.FinishPhase()
}

type readPacksStats struct {
	ui ui.ProgressUI

	packs        ui.CounterTo
	bytes, blobs int64
}

func startReadPacksProgress(pm ui.ProgressUI, packs int) *readPacksStats {
	p := &readPacksStats{ui: pm, packs: ui.StartCountTo(int64(packs))}

	progress := func() string {
		return fmt.Sprintf("Reading pack files: %d / %d pack files (%s bytes, %d blobs)",
			p.packs.Current,
			p.packs.Total,
			ui.FormatBytes(p.bytes),
			p.blobs,
		)
	}
	percent := func() (int64, int64) {
		return p.packs.Current, p.packs.Total
	}
	summary := func(time.Duration) {
		p.ui.P("Read %d pack files (%s bytes, %d blobs)",
			p.packs.Current,
			ui.FormatBytes(p.bytes),
			p.blobs,
		)
	}

	p.ui.StartPhase(progress, nil, percent, summary)

	return p
}

func (p *readPacksStats) doneReadPack(size int64, blobCnt int) {
	p.ui.Update(func() {
		p.packs.Current++
		p.bytes += size
		p.blobs += int64(blobCnt)
	})
}

func (p *readPacksStats) finish() {
	p.ui.FinishPhase()
}
