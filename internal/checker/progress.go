package checker

import "github.com/restic/restic/internal/ui"

type indexLoadStats struct {
	ui ui.ProgressUI

	totalIdxFiles, totalBlobs ui.Counter
}

func startIndexCheckProgress(ui ui.ProgressUI) *indexLoadStats {
	p := &indexLoadStats{ui: ui}

	setup := func() {}
	metrics := map[string]interface{}{
		"idxfiles": &p.totalIdxFiles,
		"blobs":    &p.totalBlobs,
	}
	progress := "{{.idxfiles.Value}} index files ({{.blobs.Value}} blobs)"
	subtotal := "Loaded {{.idxfiles.Value}} index files ({{.blobs.Value}} blobs)"

	ui.Set("Loading index files...", setup, metrics, progress, subtotal)

	return p
}

func (p *indexLoadStats) addIndexFile() {
	p.ui.Update(func() { p.totalIdxFiles.Add(1) })
}

func (p *indexLoadStats) doneIndexFile(blobCnt int) {
	p.ui.Update(func() { p.totalBlobs.Add(int64(blobCnt)) })
}

func (p *indexLoadStats) finish() {
	p.ui.Unset()
}

type packCheckStats struct {
	ui ui.ProgressUI

	totalPackFiles ui.Counter
}

func newPackCheckProgress(ui ui.ProgressUI) *packCheckStats {
	return &packCheckStats{ui: ui}
}

func (p *packCheckStats) startListPacks() {
	setup := func() {}
	metrics := map[string]interface{}{
		"packfiles": &p.totalPackFiles,
	}
	progress := "{{.packfiles.Value}} pack files"
	subtotal := "Checked {{.packfiles.Value}} pack files"

	p.ui.Set("Checking all packs...", setup, metrics, progress, subtotal)
}

func (p *packCheckStats) addPack(ui ui.ProgressUI) {
	ui.Update(func() { p.totalPackFiles.Add(1) })
}

func (p *packCheckStats) finish() {
	p.ui.Unset()
}

type structureCheckStats struct {
	ui ui.ProgressUI

	totalSnapshotFiles   ui.Counter
	checkedSnapshotFiles ui.CounterTo
}

func newStructureCheckStats(ui ui.ProgressUI) *structureCheckStats {
	return &structureCheckStats{ui: ui}
}

func (p *structureCheckStats) startLoadSnapshots() {
	setup := func() {}
	metrics := map[string]interface{}{
		"snapshots": &p.totalSnapshotFiles,
	}
	progress := "{{.snapshots.Value}} snapshot files"
	subtotal := "Loaded {{.snapshots.Value}} snapshot files"

	p.ui.Set("Loading snapshot files...", setup, metrics, progress, subtotal)
}

func (p *structureCheckStats) addSnapshot() {
	p.ui.Update(func() { p.totalSnapshotFiles.Add(1) })
}

func (p *structureCheckStats) startCheckSnapshots() {
	setup := func() {
		p.checkedSnapshotFiles = ui.StartCountTo(p.totalSnapshotFiles.Value())
	}
	metrics := map[string]interface{}{
		"snapshotFiles": &p.checkedSnapshotFiles,
		"percent":       &p.checkedSnapshotFiles,
	}
	progress := "{{.snapshotFiles.Value}} snapshots"
	subtotal := "Checked {{.snapshotFiles.Value}} snapshots"

	p.ui.Set("Checking snapshots, trees and blobs...", setup, metrics, progress, subtotal)
}

func (p *structureCheckStats) doneSnapshot() {
	p.ui.Update(func() { p.checkedSnapshotFiles.Add(1) })
}

func (p *structureCheckStats) finish() {
	p.ui.Unset()
}

type readPacksStats struct {
	ui ui.ProgressUI

	checkedPackFiles       ui.CounterTo
	totalBytes, totalBlobs ui.Counter
}

func startReadPacksProgress(pm ui.ProgressUI, packs int) *readPacksStats {
	p := &readPacksStats{ui: pm, checkedPackFiles: ui.StartCountTo(int64(packs))}

	setup := func() {}
	metrics := map[string]interface{}{
		"packfiles": &p.checkedPackFiles,
		"bytes":     &p.totalBytes,
		"blobs":     &p.totalBlobs,
		"percent":   &p.checkedPackFiles,
	}
	progress := "{{.packfiles.Value}} / {{.packfiles.Target.Value}} pack files ({{.bytes.FormatBytes}} bytes, {{.blobs.Value}} blobs)"
	subtotal := "Read {{.packfiles.Value}} pack files ({{.bytes.FormatBytes}} bytes, {{.blobs.Value}} blobs)"

	pm.Set("Reading pack files...", setup, metrics, progress, subtotal)

	return p
}

func (p *readPacksStats) doneReadPack(size int64, blobCnt int) {
	p.ui.Update(func() {
		p.checkedPackFiles.Add(1)
		p.totalBytes.Add(size)
		p.totalBlobs.Add(int64(blobCnt))
	})
}

func (p *readPacksStats) finish() {
	p.ui.Unset()
}
