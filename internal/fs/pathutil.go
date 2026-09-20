package fs

import "strings"

// splitTrailingSpaceAndDot splits off a run of trailing spaces and/or dots
// from the last path component of name, returning the remainder of the
// path (with the trailing run removed from its last component) and the
// removed run itself.
//
// This exists to work around a Windows-specific quirk (see fixpath in
// file_windows.go for the full rationale) but the string manipulation
// itself has no OS dependency, so it lives in a portable file to allow
// testing it directly on any platform.
//
// If the last component consists entirely of spaces and/or dots (e.g. ".",
// "..", "...", or " "), name is returned unchanged with an empty suffix:
// such components are navigational (current/parent directory) or otherwise
// have no literal characters to preserve, so splitting them off could
// change the meaning of the path instead of merely protecting a filename.
func splitTrailingSpaceAndDot(name string) (base string, suffix string) {
	// The last path component may be preceded by either separator, since
	// callers may pass in slash-separated paths before they reach
	// filepath.Abs.
	sepIdx := strings.LastIndexAny(name, `/\`)
	component := name[sepIdx+1:]

	trimmed := strings.TrimRight(component, " .")
	if trimmed == "" {
		// Entirely spaces/dots (or empty) - leave untouched.
		return name, ""
	}
	if len(trimmed) == len(component) {
		// Nothing to split off.
		return name, ""
	}

	suffix = component[len(trimmed):]
	base = name[:sepIdx+1] + trimmed
	return base, suffix
}
