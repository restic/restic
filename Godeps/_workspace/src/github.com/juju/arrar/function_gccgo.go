// +build gccgo

package arrar

func function(fullfunc string) string {
	// Here gccgo and golang-go differ. gccgo returns just the package and
	// name, whereas golang-go returns the full path, also gccgo returns a
	// mangled name when a receiver is involved.
	// So, don't even try at this stage.
	return ""
}
