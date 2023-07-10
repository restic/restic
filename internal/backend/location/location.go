// Package location implements parsing the restic repository location from a string.
package location

import (
	"strings"

	"github.com/restic/restic/internal/errors"
)

// Location specifies the location of a repository, including the method of
// access and (possibly) credentials needed for access.
type Location struct {
	Scheme string
	Config interface{}
}

// NoPassword returns the repository location unchanged (there's no sensitive information there)
func NoPassword(s string) string {
	return s
}

func isPath(s string) bool {
	if strings.HasPrefix(s, "../") || strings.HasPrefix(s, `..\`) {
		return true
	}

	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\`) {
		return true
	}

	if len(s) < 3 {
		return false
	}

	// check for drive paths
	drive := s[0]
	if !(drive >= 'a' && drive <= 'z') && !(drive >= 'A' && drive <= 'Z') {
		return false
	}

	if s[1] != ':' {
		return false
	}

	if s[2] != '\\' && s[2] != '/' {
		return false
	}

	return true
}

// Parse extracts repository location information from the string s. If s
// starts with a backend name followed by a colon, that backend's Parse()
// function is called. Otherwise, the local backend is used which interprets s
// as the name of a directory.
func Parse(registry *Registry, s string) (u Location, err error) {
	scheme := extractScheme(s)
	u.Scheme = scheme

	factory := registry.Lookup(scheme)
	if factory != nil {
		u.Config, err = factory.ParseConfig(s)
		if err != nil {
			return Location{}, err
		}

		return u, nil
	}

	// if s is not a path or contains ":", it's ambiguous
	if !isPath(s) && strings.ContainsRune(s, ':') {
		return Location{}, errors.New("invalid backend\nIf the repository is in a local directory, you need to add a `local:` prefix")
	}

	u.Scheme = "local"
	factory = registry.Lookup(u.Scheme)
	if factory == nil {
		return Location{}, errors.New("local backend not available")
	}

	u.Config, err = factory.ParseConfig("local:" + s)
	if err != nil {
		return Location{}, err
	}

	return u, nil
}

// StripPassword returns a displayable version of a repository location (with any sensitive information removed)
func StripPassword(registry *Registry, s string) string {
	scheme := extractScheme(s)

	factory := registry.Lookup(scheme)
	if factory != nil {
		return factory.StripPassword(s)
	}
	return s
}

func extractScheme(s string) string {
	scheme, _, _ := strings.Cut(s, ":")
	return scheme
}
