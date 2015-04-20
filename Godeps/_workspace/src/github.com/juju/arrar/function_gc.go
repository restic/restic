// +build !gccgo

package arrar

import (
	"strings"
)

func function(fullfunc string) string {
	// Here gccgo and golang-go differ.
	// gccgo returns just the package and name, whereas
	// golang-go returns the full path.
	slash := strings.LastIndex(fullfunc, "/")
	if slash > 0 {
		fullfunc = fullfunc[slash:]
	}
	return fullfunc[strings.Index(fullfunc, ".")+1:]
}
