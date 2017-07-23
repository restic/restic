package backend

import "os"

// Paths contains the default paths for file-based backends (e.g. local).
var Paths = struct {
	Data      string
	Snapshots string
	Index     string
	Locks     string
	Keys      string
	Temp      string
	Config    string
}{
	"data",
	"snapshots",
	"index",
	"locks",
	"keys",
	"tmp",
	"config",
}

// Modes holds the default modes for directories and files for file-based
// backends.
var Modes = struct{ Dir, File os.FileMode }{0700, 0600}
