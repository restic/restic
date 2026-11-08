package fuse

import (
	"strings"
	"unicode/utf8"
)

// Windows cannot represent every file name a repository may contain: the
// characters \ : * ? " < > | and control characters are not allowed, a
// trailing dot or space is removed, and a few device names are reserved.
// WinFSP hides directory entries with such names, so they are shown with
// look-alike full-width characters instead, following the convention used
// by rclone. The mapping is reversible: unescapeWindowsName(escapeWindowsName(s)) == s.

// escapeQuote marks the following rune as literal. It is used for runes of
// the original name that would otherwise be unescaped, and to prefix
// reserved device names.
const escapeQuote = '‛' // U+201B

const (
	escapedDot   = '．' // U+FF0E, replaces a trailing dot
	escapedSpace = '␠' // U+2420, replaces a trailing space

	// control characters are mapped to the "control pictures" block
	controlPictures = 0x2400
)

var escapeRunes = map[rune]rune{
	'\\': '＼', // U+FF3C
	':':  '：', // U+FF1A
	'*':  '＊', // U+FF0A
	'?':  '？', // U+FF1F
	'"':  '＂', // U+FF02
	'<':  '＜', // U+FF1C
	'>':  '＞', // U+FF1E
	'|':  '｜', // U+FF5C
}

var unescapeRunes = func() map[rune]rune {
	m := make(map[rune]rune, len(escapeRunes))
	for k, v := range escapeRunes {
		m[v] = k
	}
	return m
}()

// isEscapedRune reports whether r is produced by escaping and thus has to be
// quoted when it appears in an original name.
func isEscapedRune(r rune) bool {
	if _, ok := unescapeRunes[r]; ok {
		return true
	}
	return r == escapeQuote || r == escapedDot || r == escapedSpace ||
		(r >= controlPictures && r < controlPictures+0x20)
}

// isReservedWindowsName reports whether name, ignoring case and anything
// after the first dot, is one of the device names Windows reserves.
func isReservedWindowsName(name string) bool {
	if idx := strings.IndexByte(name, '.'); idx >= 0 {
		name = name[:idx]
	}
	name = strings.ToUpper(name)
	switch name {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(name) < 4 || (name[:3] != "COM" && name[:3] != "LPT") {
		return false
	}
	suffix := name[3:]
	isDigit := len(suffix) == 1 && suffix[0] >= '0' && suffix[0] <= '9'
	return isDigit || suffix == "¹" || suffix == "²" || suffix == "³"
}

// escapeWindowsName returns name with all parts that are not allowed in a
// Windows file name replaced by look-alike characters.
func escapeWindowsName(name string) string {
	var b strings.Builder
	if isReservedWindowsName(name) {
		b.WriteRune(escapeQuote)
	}
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		i += size
		trailing := i == len(name)
		switch {
		case r < 0x20:
			b.WriteRune(controlPictures + r)
		case trailing && r == '.':
			b.WriteRune(escapedDot)
		case trailing && r == ' ':
			b.WriteRune(escapedSpace)
		case isEscapedRune(r):
			b.WriteRune(escapeQuote)
			b.WriteRune(r)
		default:
			if e, ok := escapeRunes[r]; ok {
				r = e
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unescapeWindowsName reverses escapeWindowsName. Names that were not
// produced by escapeWindowsName are returned as close to unchanged as possible.
func unescapeWindowsName(name string) string {
	var b strings.Builder
	quoted := false
	for _, r := range name {
		switch {
		case quoted:
			b.WriteRune(r)
			quoted = false
		case r == escapeQuote:
			quoted = true
		case r == escapedDot:
			b.WriteRune('.')
		case r == escapedSpace:
			b.WriteRune(' ')
		case r >= controlPictures && r < controlPictures+0x20:
			b.WriteRune(r - controlPictures)
		default:
			if o, ok := unescapeRunes[r]; ok {
				r = o
			}
			b.WriteRune(r)
		}
	}
	if quoted {
		b.WriteRune(escapeQuote)
	}
	return b.String()
}
