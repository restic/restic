// +build !windows

package termstatus

import (
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

// On Unix, real terminals are always POSIX terminals.
func (t *Terminal) clearCurrentLine()  { posixClearCurrentLine(t.wr) }
func (t *Terminal) moveCursorUp(n int) { posixMoveCursorUp(t.wr, n) }

// initTermType sets t.termType and, if t is a terminal, t.fd.
func (t *Terminal) initTermType(fd int) {
	if !terminal.IsTerminal(fd) {
		return
	}
	term := os.Getenv("TERM")
	// TODO actually read termcap db and detect if terminal supports what we need
	if term == "" || term == "dumb" {
		return
	}

	t.fd = fd
	t.termType = termTypePosix
}
