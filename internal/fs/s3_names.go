package fs

import (
	"fmt"
	"strings"
)

// Object keys in S3 are arbitrary strings which only use "/" by convention to
// form a hierarchy. Keys can contain empty path components ("a//b"), the
// components "." and "..", and an object can have the same name as a
// "directory" ("a" and "a/b"). None of these can be represented as is in a
// restic tree, so each path component is encoded using '¤' (U+00A4):
//
//   - '¤' is encoded as "¤A4"
//   - the empty component is encoded as "¤"
//   - the components "." and ".." are encoded as "¤2E" and "¤2E¤2E"
//   - an object which has the same name as a directory gets a "¤" suffix
//
// All other characters are kept as is. The hex digits are the Unicode code
// point of the encoded character.

const (
	s3Escape       = "¤"
	s3EscapeEscape = "¤A4"
	s3ShadowSuffix = s3Escape
)

// s3EncodeComponent encodes a single path component of an object key.
func s3EncodeComponent(s string) string {
	switch s {
	case "":
		return s3Escape
	case ".":
		return "¤2E"
	case "..":
		return "¤2E¤2E"
	}
	return strings.ReplaceAll(s, s3Escape, s3EscapeEscape)
}

// s3EncodeObjectName encodes the last path component of an object key. If a
// directory with the same name exists, shadowed must be set.
func s3EncodeObjectName(s string, shadowed bool) string {
	name := s3EncodeComponent(s)
	if shadowed {
		name += s3ShadowSuffix
	}
	return name
}

// s3DecodeName reverses s3EncodeObjectName. shadowed reports whether the name
// carried the suffix for objects which have the same name as a directory.
// Names which s3EncodeObjectName cannot produce are rejected, so that each
// key maps to exactly one name.
func s3DecodeName(name string) (s string, shadowed bool, err error) {
	enc := name
	if name != s3Escape && strings.HasSuffix(name, s3ShadowSuffix) {
		shadowed = true
		enc = strings.TrimSuffix(name, s3ShadowSuffix)
	}

	switch enc {
	case s3Escape:
		s = ""
	case "¤2E":
		s = "."
	case "¤2E¤2E":
		s = ".."
	default:
		s = strings.ReplaceAll(enc, s3EscapeEscape, s3Escape)
	}

	if s3EncodeObjectName(s, shadowed) != name {
		return "", false, fmt.Errorf("invalid encoded name %q", name)
	}
	return s, shadowed, nil
}
