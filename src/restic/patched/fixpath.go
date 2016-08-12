package patched

import (
	"runtime"
)

func Fixpath(name string) string {
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + name
	} else {
		return name
	}
}

