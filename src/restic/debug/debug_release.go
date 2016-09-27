// +build !debug

package debug

// Log prints a message to the debug log (if debug is enabled).
func Log(tag string, fmt string, args ...interface{}) {}
