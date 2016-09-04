package sftp

import (
	"net/url"
	"path"
	"strings"

	"restic/errors"
)

// Config collects all information required to connect to an sftp server.
type Config struct {
	User, Host, Dir string
}

// ParseConfig parses the string s and extracts the sftp config. The
// supported configuration formats are sftp://user@host/directory
// (with an optional port sftp://user@host:port/directory) and
// sftp:user@host:directory.  The directory will be path Cleaned and
// can be an absolute path if it starts with a '/'
// (e.g. sftp://user@host//absolute and sftp:user@host:/absolute).
func ParseConfig(s string) (interface{}, error) {
	var user, host, dir string
	switch {
	case strings.HasPrefix(s, "sftp://"):
		// parse the "sftp://user@host/path" url format
		url, err := url.Parse(s)
		if err != nil {
			return nil, errors.Wrap(err, "url.Parse")
		}
		if url.User != nil {
			user = url.User.Username()
		}
		host = url.Host
		dir = url.Path
		if dir == "" {
			return nil, errors.Errorf("invalid backend %q, no directory specified", s)
		}

		dir = dir[1:]
	case strings.HasPrefix(s, "sftp:"):
		// parse the sftp:user@host:path format, which means we'll get
		// "user@host:path" in s
		s = s[5:]
		// split user@host and path at the colon
		data := strings.SplitN(s, ":", 2)
		if len(data) < 2 {
			return nil, errors.New("sftp: invalid format, hostname or path not found")
		}
		host = data[0]
		dir = data[1]
		// split user and host at the "@"
		data = strings.SplitN(host, "@", 2)
		if len(data) == 2 {
			user = data[0]
			host = data[1]
		}
	default:
		return nil, errors.New(`invalid format, does not start with "sftp:"`)
	}
	return Config{
		User: user,
		Host: host,
		Dir:  path.Clean(dir),
	}, nil
}
