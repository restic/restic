package sftp

import (
	"net/url"
	"path"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config collects all information required to connect to an sftp server.
type Config struct {
	User, Host, Port, Path string

	Layout  string `option:"layout" help:"use this backend directory layout (default: auto-detect)"`
	Command string `option:"command" help:"specify command to create sftp connection"`

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

// NewConfig returns a new config with default options applied.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

func init() {
	options.Register("sftp", Config{})
}

// ParseConfig parses the string s and extracts the sftp config. The
// supported configuration formats are sftp://user@host[:port]/directory
// and sftp:user@host:directory.  The directory will be path Cleaned and can
// be an absolute path if it starts with a '/' (e.g.
// sftp://user@host//absolute and sftp:user@host:/absolute).
func ParseConfig(s string) (interface{}, error) {
	var user, host, port, dir string
	switch {
	case strings.HasPrefix(s, "sftp://"):
		// parse the "sftp://user@host/path" url format
		url, err := url.Parse(s)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if url.User != nil {
			user = url.User.Username()
		}
		host = url.Hostname()
		port = url.Port()
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
		var colon bool
		host, dir, colon = strings.Cut(s, ":")
		if !colon {
			return nil, errors.New("sftp: invalid format, hostname or path not found")
		}
		// split user and host at the "@"
		data := strings.SplitN(host, "@", 3)
		if len(data) == 3 {
			user = data[0] + "@" + data[1]
			host = data[2]
		} else if len(data) == 2 {
			user = data[0]
			host = data[1]
		}
	default:
		return nil, errors.New(`invalid format, does not start with "sftp:"`)
	}

	p := path.Clean(dir)
	if strings.HasPrefix(p, "~") {
		return nil, errors.Fatal("sftp path starts with the tilde (~) character, that fails for most sftp servers.\nUse a relative directory, most servers interpret this as relative to the user's home directory.")
	}

	cfg := NewConfig()
	cfg.User = user
	cfg.Host = host
	cfg.Port = port
	cfg.Path = p

	return cfg, nil
}
