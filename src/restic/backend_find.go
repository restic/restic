package restic

import "restic/errors"

// ErrNoIDPrefixFound is returned by Find() when no ID for the given prefix
// could be found.
var ErrNoIDPrefixFound = errors.New("no ID found")

// ErrMultipleIDMatches is returned by Find() when multiple IDs with the given
// prefix are found.
var ErrMultipleIDMatches = errors.New("multiple IDs with prefix found")

// Find loads the list of all files of type t and searches for names which
// start with prefix. If none is found, nil and ErrNoIDPrefixFound is returned.
// If more than one is found, nil and ErrMultipleIDMatches is returned.
func Find(be Lister, t FileType, prefix string) (string, error) {
	done := make(chan struct{})
	defer close(done)

	match := ""

	// TODO: optimize by sorting list etc.
	for name := range be.List(t, done) {
		if prefix == name[:len(prefix)] {
			if match == "" {
				match = name
			} else {
				return "", ErrMultipleIDMatches
			}
		}
	}

	if match != "" {
		return match, nil
	}

	return "", ErrNoIDPrefixFound
}

const minPrefixLength = 8

// PrefixLength returns the number of bytes required so that all prefixes of
// all names of type t are unique.
func PrefixLength(be Lister, t FileType) (int, error) {
	done := make(chan struct{})
	defer close(done)

	// load all IDs of the given type
	list := make([]string, 0, 100)
	for name := range be.List(t, done) {
		list = append(list, name)
	}

	// select prefixes of length l, test if the last one is the same as the current one
	id := ID{}
outer:
	for l := minPrefixLength; l < len(id); l++ {
		var last string

		for _, name := range list {
			if last == name[:l] {
				continue outer
			}
			last = name[:l]
		}

		return l, nil
	}

	return len(id), nil
}
