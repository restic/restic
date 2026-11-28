package main

import (
	"strconv"
	"testing"

	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

func TestPrintTextDiffChangeQuotesPath(t *testing.T) {
	term := &ui.MockTerminal{}
	printer := progress.NewTerminalPrinter(false, 1, term)
	printTextDiffChange(printer.S, NewChange("dir\nfile", "+"))
	rtest.Equals(t, []string{"+    " + strconv.Quote("dir\nfile")}, term.Output)

	term.Output = nil
	printTextDiffChange(printer.S, NewChange("/ordinary/file", "+"))
	rtest.Equals(t, []string{"+    /ordinary/file"}, term.Output)
}
