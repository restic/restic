package terminal

import (
	"os"

	"golang.org/x/term"
)

func StdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func StdoutIsTerminal() bool {
	// mintty on windows can use pipes which behave like a posix terminal,
	// but which are not a terminal handle
	return term.IsTerminal(int(os.Stdout.Fd())) || StdoutCanUpdateStatus()
}

func StdoutCanUpdateStatus() bool {
	return CanUpdateStatus(os.Stdout.Fd())
}

func StdoutWidth() int {
	return Width(os.Stdout.Fd())
}

func Width(fd uintptr) int {
	w, _, err := term.GetSize(int(fd))
	if err != nil {
		return 0
	}
	return w
}
