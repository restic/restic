package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/restic/restic/internal/archiver"
)

// HTTPRestore contains the state of the HTTP reporter for restoration
type HTTPRestore struct {
	enabled   bool
	State     int
	r         *Restore
	ScanStats archiver.ScanStats
	Snapshot  string
	startTime time.Time

	url      string
	interval int
	token    string
}

func (h HTTPRestore) send(message httpMessage) {
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

func (h HTTPRestore) newMessage() httpMessage {
	msg := httpMessage{
		Token:     h.token,
		Action:    "restore",
		StartTime: h.startTime,
		Snapshot:  h.Snapshot,
	}
	switch h.State {
	case HTTPNone:
		msg.Status = "none" // Should never occur
	case HTTPReadingIndex:
		msg.Status = "indexing"
	case HTTPScanningData:
		msg.Status = "scanning"
	case HTTPDoingRestore:
		msg.Status = "doing_restore"
	case HTTPDone:
		msg.Status = "done"
	default:
		panic("Unexpected state " + strconv.Itoa(h.State))
	}
	msg.Successful = true
	msg.ErrorMsg = ""
	return msg
}

func (h HTTPRestore) Error(err error) {
	if !h.enabled {
		return
	}
	msg := h.newMessage()
	msg.Successful = false
	msg.ErrorMsg = err.Error()
	h.send(msg)
}

// SendUpdate sends a message with the current statistics.
func (h HTTPRestore) SendUpdate() {
	if !h.enabled {
		return
	}
	msg := h.newMessage()
	msg.SecsElapsed = int64(time.Since(h.r.start).Seconds())
	if h.State == HTTPScanningData {
		msg.FilesProcessed = h.ScanStats.Files
		msg.BytesProcessed = h.ScanStats.Bytes
	} else {
		msg.FilesProcessed = h.r.processed.Files
		msg.BytesProcessed = h.r.processed.Bytes
	}
	msg.NumErrors = h.r.errors
	if (h.r.total.Files != 0 || h.r.total.Dirs != 0) && h.r.eta > 0 && h.r.processed.Bytes < h.r.total.Bytes {
		msg.HasETA = true
		msg.ETA = h.r.eta
	}
	h.send(msg)
}

// SendDone reports completion, sending a message with the final statistics.
func (h *HTTPRestore) SendDone() {
	if !h.enabled {
		return
	}
	h.State = HTTPDone
	msg := h.newMessage()
	h.State = HTTPNone
	msg.FilesNew = h.r.summary.Files.New
	msg.FilesChanged = h.r.summary.Files.Changed
	msg.FilesUnmodified = h.r.summary.Files.Unchanged
	/*
		msg.DirsNew = h.r.summary.Dirs.New
		msg.DirsChanged = h.r.summary.Dirs.Changed
		msg.DirsUnmodified = h.r.summary.Dirs.Unchanged
	*/
	msg.SecsElapsed = int64(time.Since(h.r.start).Seconds())
	msg.FilesProcessed = h.r.processed.Files
	msg.BytesProcessed = h.r.processed.Bytes
	msg.NumErrors = h.r.errors
	h.send(msg)
}

// NewHTTPRestore creates an HTTP reporter for restoring
func NewHTTPRestore(r *Restore, url string, interval int, token string) *HTTPRestore {
	if url == "" {
		return &HTTPRestore{}
	}
	ticker := time.NewTicker(time.Duration(interval * int(time.Second)))
	instance := HTTPRestore{
		enabled:   true,
		url:       url,
		interval:  interval,
		token:     token,
		r:         r,
		startTime: time.Now(),
	}
	go func() {
		for range ticker.C {
			instance.SendUpdate()
		}
	}()
	return &instance
}
