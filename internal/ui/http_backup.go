package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/restic/restic/internal/archiver"
)

// HTTPBackup contains the state of the HTTP reporter for backup
type HTTPBackup struct {
	enabled   bool
	State     int
	b         *Backup
	ScanStats archiver.ScanStats
	startTime time.Time

	url      string
	interval int
	token    string
}

func (h HTTPBackup) send(message httpMessage) {
	if message.Status == "none" {
		return
	}
	j, err := json.Marshal(message)
	if err != nil {
		panic(err)
	}
	_, err = http.Post(h.url, "text/json", bytes.NewReader(j))
	if err != nil {
		panic(err)
	}
}

func (h HTTPBackup) newMessage() httpMessage {
	msg := httpMessage{
		Token:     h.token,
		Action:    "backup",
		StartTime: h.startTime,
	}
	switch h.State {
	case HTTPNone:
		msg.Status = "none" // Should never occur
	case HTTPReadingIndex:
		msg.Status = "indexing"
	case HTTPScanningData:
		msg.Status = "scanning"
	case HTTPDoingBackup:
		msg.Status = "doing_backup"
	case HTTPDone:
		msg.Status = "done"
	default:
		panic("Unexpected state " + strconv.Itoa(h.State))
	}
	msg.Successful = true
	msg.ErrorMsg = ""
	return msg
}

func (h HTTPBackup) Error(err error) {
	if !h.enabled {
		return
	}
	msg := h.newMessage()
	msg.Successful = false
	msg.ErrorMsg = err.Error()
	h.send(msg)
}

// SendUpdate sends a message with the current statistics.
func (h HTTPBackup) SendUpdate() {
	if !h.enabled {
		return
	}
	msg := h.newMessage()
	msg.SecsElapsed = int64(time.Since(h.b.start).Seconds())
	if h.State == HTTPScanningData {
		msg.FilesProcessed = h.ScanStats.Files
		msg.BytesProcessed = h.ScanStats.Bytes
	} else {
		msg.FilesProcessed = h.b.processed.Files
		msg.BytesProcessed = h.b.processed.Bytes
	}
	msg.NumErrors = h.b.errors
	if (h.b.total.Files != 0 || h.b.total.Dirs != 0) && h.b.eta > 0 && h.b.processed.Bytes < h.b.total.Bytes {
		msg.HasETA = true
		msg.ETA = h.b.eta
	}
	h.send(msg)
}

// SendDone reports completion, sending a message with the final statistics.
func (h *HTTPBackup) SendDone(snapshot string) {
	if !h.enabled {
		return
	}
	h.State = HTTPDone
	msg := h.newMessage()
	h.State = HTTPNone
	msg.Snapshot = snapshot
	msg.FilesNew = h.b.summary.Files.New
	msg.FilesChanged = h.b.summary.Files.Changed
	msg.FilesUnmodified = h.b.summary.Files.Unchanged
	msg.DirsNew = h.b.summary.Dirs.New
	msg.DirsChanged = h.b.summary.Dirs.Changed
	msg.DirsUnmodified = h.b.summary.Dirs.Unchanged
	msg.SecsElapsed = int64(time.Since(h.b.start).Seconds())
	msg.FilesProcessed = h.b.processed.Files
	msg.BytesProcessed = h.b.processed.Bytes
	msg.NumErrors = h.b.errors
	h.send(msg)
}

// NewHTTPBackup creates an HTTP reporter for backup
func NewHTTPBackup(b *Backup, url string, interval int, token string) *HTTPBackup {
	if url == "" {
		return &HTTPBackup{}
	}
	ticker := time.NewTicker(time.Duration(interval * int(time.Second)))
	instance := HTTPBackup{
		enabled:   true,
		url:       url,
		interval:  interval,
		token:     token,
		b:         b,
		startTime: time.Now(),
	}
	go func() {
		for range ticker.C {
			instance.SendUpdate()
		}
	}()
	return &instance
}
