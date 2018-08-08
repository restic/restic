package restic

import (
	"context"

	"github.com/restic/restic/internal/errors"
)

// ErrNoIDPrefixFound is returned by Find() when no ID for the given prefix
// could be found.
var ErrNoIDPrefixFound = errors.New("no matching ID found")

// ErrMultipleIDMatches is returned by Find() when multiple IDs with the given
// prefix are found.
var ErrMultipleIDMatches = errors.New("multiple IDs with prefix found")

// Find loads the list of all files of type t and searches for names which
// start with prefix. If none is found, nil and ErrNoIDPrefixFound is returned.
// If more than one is found, nil and ErrMultipleIDMatches is returned.
func Find(be Lister, t FileType, prefix string) (string, error) {
	match := ""

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	err := be.List(ctx, t, func(fi FileInfo) error {
		if prefix == fi.Name[:len(prefix)] {
			if match == "" {
				match = fi.Name
			} else {
				return ErrMultipleIDMatches
			}
		}

		return nil
	})

	if err != nil {
		return "", err
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
	// load all IDs of the given type
	list := make([]string, 0, 100)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	err := be.List(ctx, t, func(fi FileInfo) error {
		list = append(list, fi.Name)
		return nil
	})

	if err != nil {
		return 0, err
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
