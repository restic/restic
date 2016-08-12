package patched

import (
	"path/filepath"
	"runtime"
)

func Fixpath(name string) string {
	if runtime.GOOS == "windows" {
		abspath, err := filepath.Abs(name)
		if err == nil {
			return "\\\\?\\" + abspath
		} 
	}
	return name
}

