package backend

import "os"

// Default paths for file-based backends (e.g. local)
var Paths = struct {
	Data      string
	Snapshots string
	Trees     string
	Index     string
	Locks     string
	Keys      string
	Temp      string
	Version   string
	ID        string
}{
	"data",
	"snapshots",
	"trees",
	"index",
	"locks",
	"keys",
	"tmp",
	"version",
	"id",
}

// Default modes for file-based backends
var Modes = struct{ Dir, File os.FileMode }{0700, 0600}
