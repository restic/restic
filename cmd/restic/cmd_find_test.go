package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/restic/restic/internal/data"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

func TestPrintObjectQuotesPath(t *testing.T) {
	term := &ui.MockTerminal{}
	out := &statefulOutput{
		printer: progress.NewTerminalPrinter(false, 1, term),
	}
	out.PrintObjectNormal("file", "abcd", "dir\nfile", "", &data.Snapshot{})

	rtest.Equals(t, " ... path "+strconv.Quote("dir\nfile"), term.Output[1])
	for _, line := range term.Output {
		rtest.Assert(t, !strings.Contains(line, "\n"), "raw newline in %q", line)
	}

	term = &ui.MockTerminal{}
	out.printer = progress.NewTerminalPrinter(false, 1, term)
	out.PrintObjectNormal("blob", "abcd", "/ordinary/file", "tree", &data.Snapshot{})
	rtest.Equals(t, " ... in file /ordinary/file", term.Output[1])
}
