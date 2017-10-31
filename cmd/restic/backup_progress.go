package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"text/template"
	"time"
)

// ProgressAction type of progress output
type ProgressAction string

const (
	// ProgressTypeScanner scanner progress type
	ProgressTypeScanner ProgressAction = "scanner"
	// ProgressTypeArchive archive process progress type
	ProgressTypeArchive ProgressAction = "archive"
)

type (
	// ProgressStatus provides json archive process progress
	ProgressStatus struct {
		ProgressType   ProgressAction `json:"type"`
		Duration       time.Duration  `json:"duration"`
		Directories    uint64         `json:"directories"`
		Files          uint64         `json:"files"`
		Completed      uint64         `json:"completed"`
		Total          uint64         `json:"total"`
		Bps            uint64         `json:"bps"`
		CompletedBytes uint64         `json:"completed_bytes"`
		ItemsDone      uint64         `json:"items_done"`
		ItemsTotal     uint64         `json:"items_total"`
		TotalBytes     uint64         `json:"total_bytes"`
		ETA            uint64         `json:"eta"`
		Errors         uint64         `json:"errors"`
		Seconds        uint64         `json:"seconds"`
	}
)

// Templates for outputting current status
const (
	ScanProgressUpdateTemplate      = `[{{duration .Duration}}] {{.Directories}} directories, {{.Files}} files, {{bytes .TotalBytes}}`
	ScanProgressDoneTemplate        = `scanned {{.Directories}} directories, {{.Files}} files in {{duration .Duration}}`
	ArchiveProgressUpdateTemplate   = `[{{duration .Duration}}] {{percent .Completed .Total}}  {{bytes .Bps}}/s  {{bytes .CompletedBytes}} / {{bytes .TotalBytes}}  {{.ItemsDone}} / {{.ItemsTotal}} items  {{.Errors}} errors  `
	ArchiveProgressEstimateTemplate = `ETA {{seconds .ETA}}`
	ArchiveProgressDoneTemplate     = `duration {{duration .Duration}}, {{rate .TotalBytes .Duration}}`
)

var scanProgressUpdateTemplate,
	scanProgressDoneTemplate,
	archiveProgressTemplate,
	archiveProgresEstimateTemplate,
	archiveProgressDoneTemplate *template.Template

func init() {
	templateFunctions := template.FuncMap{
		"duration": formatDuration,
		"percent":  formatPercent,
		"bytes":    formatBytes,
		"seconds":  formatSeconds,
		"rate":     formatRate,
	}

	scanProgressUpdateTemplate = template.Must(template.New("scanProgressUpdateTemplate").Funcs(templateFunctions).Parse(ScanProgressUpdateTemplate))
	scanProgressDoneTemplate = template.Must(template.New("scanProgressDoneTemplate").Funcs(templateFunctions).Parse(ScanProgressDoneTemplate))
	archiveProgressTemplate = template.Must(template.New("archiveProgressTemplate").Funcs(templateFunctions).Parse(ArchiveProgressUpdateTemplate))
	archiveProgresEstimateTemplate = template.Must(template.New("archiveProgressEstimateTemplate").Funcs(templateFunctions).Parse(ArchiveProgressEstimateTemplate))
	archiveProgressDoneTemplate = template.Must(template.New("archiveProgressTemplate").Funcs(templateFunctions).Parse(ArchiveProgressDoneTemplate))
}

// NewProgressStatus return ProgressStatus struct from scanner functions
func NewProgressStatus(Duration time.Duration, Directories, Files, TotalBytes uint64) *ProgressStatus {
	return &ProgressStatus{
		ProgressType: ProgressTypeScanner,
		Duration:     Duration,
		Directories:  Directories,
		Files:        Files,
		TotalBytes:   TotalBytes,
	}
}

// UpdateScanStatus return ProgressStatus struct from scanner functions
func (ps *ProgressStatus) UpdateScanStatus(Duration time.Duration, Directories, Files, TotalBytes uint64) {
	ps.ProgressType = ProgressTypeScanner
	ps.Duration = Duration
	ps.Directories = Directories
	ps.Files = Files
	ps.TotalBytes = TotalBytes
	ps.Seconds = uint64(Duration / time.Second)
}

// UpdateProgressStatus update current progress struct with recent data
func (ps *ProgressStatus) UpdateProgressStatus(Duration time.Duration,
	Completed, Total, Bps, CompletedBytes uint64,
	TotalBytes, ItemsDone, ItemsTotal, ETA, Errors uint64) {

	ps.ProgressType = ProgressTypeArchive
	ps.Duration = Duration

	ps.Completed = Completed
	ps.Total = Total
	ps.Bps = Bps

	ps.CompletedBytes = CompletedBytes

	ps.TotalBytes = TotalBytes
	ps.ItemsDone = ItemsDone
	ps.ItemsTotal = ItemsTotal
	ps.Seconds = uint64(Duration / time.Second)
}

// PrintJSON output of current status
func (ps *ProgressStatus) PrintJSON() {
	js, err := json.Marshal(ps)

	if err != nil {
		msg := fmt.Sprintf("%s\n", err)
		Warnf(msg)
		Exitf(100, msg)
	}

	f := bufio.NewWriter(os.Stdout)
	fmt.Fprintf(f, "%s\n", js)
	f.Flush()
}

// PrintScannerProgress print the scanner content
func (ps *ProgressStatus) PrintScannerProgress() {
	var result bytes.Buffer
	err := scanProgressUpdateTemplate.Execute(&result, ps)
	if err != nil {
		panic(err)
	}
	PrintProgress(result.String())
}

// PrintScannerDone will print the scanner done content
func (ps *ProgressStatus) PrintScannerDone() {
	var result bytes.Buffer
	err := scanProgressDoneTemplate.Execute(&result, ps)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\n%s\n", result.String())
}

// PrintArchiveProgress will print current archive progress
func (ps *ProgressStatus) PrintArchiveProgress() {
	var progressString bytes.Buffer
	var etaString bytes.Buffer

	err := archiveProgressTemplate.Execute(&progressString, ps)

	if err != nil {
		panic(err)
	}

	err = archiveProgresEstimateTemplate.Execute(&etaString, ps)

	if err != nil {
		panic(err)
	}

	PrintProgress("%s%s", sliceForTerminalWidth(progressString.String()), etaString.String())
}

func sliceForTerminalWidth(status1 string) string {
	if w := stdoutTerminalWidth(); w > 0 {
		maxlen := w - len(status1)

		if maxlen < 4 {
			status1 = ""
		} else if len(status1) > maxlen {
			status1 = status1[:maxlen-4]
			status1 += "... "
		}
	}

	return status1
}

// PrintArchiveDoneProgress will print current archive progress
func (ps *ProgressStatus) PrintArchiveDoneProgress() {
	var result bytes.Buffer
<<<<<<< HEAD
	err := archiveProgressDoneTemplate.Execute(&result, ps)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\n%s\n", result.String())
=======
	_ = archiveProgressDoneTemplate.Execute(&result, ps)
	// PrintProgress(result.String())
	fmt.Printf("\n%s", result.String())
>>>>>>> f32c7f4bf2adafaec2a36275568dc8f67bcbd914
}
