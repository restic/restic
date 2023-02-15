package smb

import (
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to an SMB server
type Config struct {
	Host      string
	Port      int
	ShareName string
	Path      string

	User     string               `option:"user" help:"specify the SMB user for NTLM authentication."`
	Password options.SecretString `option:"password" help:"specify the SMB password for NTLM authentication."`
	Domain   string               `option:"domain" help:"specify the domain for authentication."`

	Layout                string        `option:"layout" help:"use this backend directory layout (default: auto-detect)"`
	Connections           uint          `option:"connections" help:"set a limit for the number of concurrent operations (default: 5)"`
	IdleTimeout           time.Duration `option:"idle-timeout" help:"Max time in seconds before closing idle connections. If no connections have been returned to the connection pool in the time given, the connection pool will be emptied. Set to 0 to keep connections indefinitely.(default: 60)"`
	RequireMessageSigning bool          `option:"require-message-signing" help:"Mandates message signing otherwise does not allow the connection. If this is false, messaging signing is just enabled and not enforced. (default: false)"`
	Dialect               uint16        `option:"dialect" help:"Force a specific dialect to be used. For SMB311 use '785', for SMB302 use '770', for SMB300 use '768', for SMB210 use '528', for SMB202 use '514', for SMB2 use '767'. If unspecfied (0), following dialects are tried in order - SMB311, SMB302, SMB300, SMB210, SMB202 (default: 0)"`
	ClientGUID            string        `option:"client-guid" help:"A 16-byte GUID to uniquely identify a client. If not specific a random GUID is used. (default: \"\")"`
}

const (
	DefaultSmbPort     int           = 445              // DefaultSmbPort returns the default port for SMB
	DefaultDomain      string        = "WORKGROUP"      // DefaultDomain returns the default domain for SMB
	DefaultConnections uint          = 5                // DefaultConnections returns the number of concurrent connections for SMB.
	DefaultIdleTimeout time.Duration = 60 * time.Second // DefaultIdleTimeout returns the default max time before closing idle connections for SMB.
)

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Port:        DefaultSmbPort,
		Domain:      DefaultDomain,
		IdleTimeout: DefaultIdleTimeout,
		Connections: DefaultConnections,
	}
}

func init() {
	options.Register("smb", Config{})
}

// ParseConfig parses the string s and extracts the s3 config. The
// supported configuration format is smb://[user@]host[:port]/sharename/directory.
// User and port are optional. Default port is 445.
func ParseConfig(s string) (interface{}, error) {
	var repo string
	switch {
	case strings.HasPrefix(s, "smb://"):
		repo = s
	case strings.HasPrefix(s, "smb:"):
		repo = "smb://" + s[4:]
	default:
		return nil, errors.New("smb: invalid format")
	}
	var user, host, port, dir string

	// parse the "smb://user@host/sharename/directory." url format
	url, err := url.Parse(repo)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if url.User != nil {
		user = url.User.Username()
		//Intentionally not allowing passwords to be set in url as
		//it can cause issues when passwords have special characters
		//like '@' and it is not recommended to pass passwords in the url.
	}

	host = url.Hostname()
	if host == "" {
		return nil, errors.New("smb: invalid format, host name not found")
	}
	port = url.Port()
	dir = url.Path
	if dir == "" {
		return nil, errors.Errorf("smb: invalid format, sharename/directory not found")
	}

	dir = dir[1:]

	var portNum int
	if port == "" {
		portNum = DefaultSmbPort
	} else {
		var err error
		portNum, err = strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
	}

	sharename, directory, _ := strings.Cut(dir, "/")

	return createConfig(user, host, portNum, sharename, directory)
}

func createConfig(user string, host string, port int, sharename, directory string) (interface{}, error) {
	if host == "" {
		return nil, errors.New("smb: invalid format, Host not found")
	}

	if directory != "" {
		directory = path.Clean(directory)
	}

	cfg := NewConfig()
	cfg.User = user
	cfg.Host = host
	cfg.Port = port
	cfg.ShareName = sharename
	cfg.Path = directory
	return cfg, nil
}
