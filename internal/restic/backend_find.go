package restic

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/debug"
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
func Find(ctx context.Context, be Lister, t FileType, prefix string) (ID, error) {
	match := ID{}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	err := be.List(ctx, t, func(fi FileInfo) error {
		// ignore filename which are not an id
		id, err := ParseID(fi.Name)
		if err != nil {
			debug.Log("unable to parse %v as an ID", fi.Name)
			return nil
		}

		if len(fi.Name) >= len(prefix) && prefix == fi.Name[:len(prefix)] {
			if match.IsNull() {
				match = id
			} else {
				return &MultipleIDMatchesError{prefix}
			}
		}

		return nil
	})

	if err != nil {
		return ID{}, err
	}

	if !match.IsNull() {
		return match, nil
	}

	return ID{}, &NoIDByPrefixError{prefix}
}
