package restic

import (
	"context"
	"fmt"
)

// A MultipleIDMatchesError is returned by Find() when multiple IDs with a
// given prefix are found.
type MultipleIDMatchesError struct{ prefix string }

func (e *MultipleIDMatchesError) Error() string {
	return fmt.Sprintf("multiple IDs with prefix %q found", e.prefix)
}

// A NoIDByPrefixError is returned by Find() when no ID for a given prefix
// could be found.
type NoIDByPrefixError struct{ prefix string }

func (e *NoIDByPrefixError) Error() string {
	return fmt.Sprintf("no matching ID found for prefix %q", e.prefix)
}

// Find loads the list of all files of type t and searches for names which
// start with prefix. If none is found, nil and ErrNoIDPrefixFound is returned.
// If more than one is found, nil and ErrMultipleIDMatches is returned.
func Find(ctx context.Context, be Lister, t FileType, prefix string) (string, error) {
	match := ""

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	err := be.List(ctx, t, func(fi FileInfo) error {
		if len(fi.Name) >= len(prefix) && prefix == fi.Name[:len(prefix)] {
			if match == "" {
				match = fi.Name
			} else {
				return &MultipleIDMatchesError{prefix}
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

	return "", &NoIDByPrefixError{prefix}
}

const minPrefixLength = 8

// PrefixLength returns the number of bytes required so that all prefixes of
// all names of type t are unique.
func PrefixLength(ctx context.Context, be Lister, t FileType) (int, error) {
	// load all IDs of the given type
	list := make([]string, 0, 100)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	err := be.List(ctx, t, func(fi FileInfo) error {
		list = append(list, fi.Name)
		return nil
	})

	if err != nil {
		return 0, err
	}

	// select prefixes of length l, test if the last one is the same as the current one
	var id ID
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
