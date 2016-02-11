// Package location implements parsing the restic repository location from a string.
package location

import (
	"strings"

	"github.com/restic/restic/backend/gcs"
	"github.com/restic/restic/backend/local"
	"github.com/restic/restic/backend/s3"
	"github.com/restic/restic/backend/sftp"
)

// Location specifies the location of a repository, including the method of
// access and (possibly) credentials needed for access.
type Location struct {
	Scheme string
	Config interface{}
}

type parser struct {
	scheme string
	parse  func(string) (interface{}, error)
}

// parsers is a list of valid config parsers for the backends. The first parser
// is the fallback and should always be set to the local backend.
var parsers = []parser{
	{local.Scheme, local.ParseConfig},
	{sftp.Scheme, sftp.ParseConfig},
	{s3.Scheme, s3.ParseConfig},
	{gcs.Scheme, gcs.ParseConfig},
}

// Parse extracts repository location information from the string s. If s
// starts with a backend name followed by a colon, that backend's Parse()
// function is called. Otherwise, the local backend is used which interprets s
// as the name of a directory.
func Parse(s string) (u Location, err error) {
	scheme := extractScheme(s)
	u.Scheme = scheme

	for _, parser := range parsers {
		if parser.scheme != scheme {
			continue
		}

		u.Config, err = parser.parse(s)
		if err != nil {
			return Location{}, err
		}

		return u, nil
	}

	// try again, with the local parser and the prefix "local:"
	u.Scheme = local.Scheme
	u.Config, err = local.ParseConfig(local.Scheme + ":" + s)
	if err != nil {
		return Location{}, err
	}

	return u, nil
}

func extractScheme(s string) string {
	data := strings.SplitN(s, ":", 2)
	return data[0]
}
