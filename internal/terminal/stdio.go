package terminal

import (
	"golang.org/x/term"
)

func InputIsTerminal(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

func OutputIsTerminal(fd uintptr) bool {
	// mintty on windows can use pipes which behave like a posix terminal,
	// but which are not a terminal handle. Thus also check `CanUpdateStatus`,
	// which is able to detect such pipes.
	return term.IsTerminal(int(fd)) || CanUpdateStatus(fd)
}

func Width(fd uintptr) int {
	w, _, err := term.GetSize(int(fd))
	if err != nil {
		return 0
	}
	return w
}
