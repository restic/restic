// +build !debug

package restic

func debug(tag string, fmt string, args ...interface{}) {}

func debug_break(string) {}
