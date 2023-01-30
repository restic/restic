package smb

import (
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to an SMB server
type Config struct {
	Address   string
	Port      int
	ShareName string
	Path      string

	Layout                string         `option:"layout" help:"use this backend directory layout (default: auto-detect)"`
	Connections           uint           `option:"connections" help:"set a limit for the number of concurrent operations (default: 2)"`
	IdleTimeout           *time.Duration `option:"idle-timeout" help:"Max time in seconds before closing idle connections. If no connections have been returned to the connection pool in the time given, the connection pool will be emptied. Set to 0 to keep connections indefinitely.(default: 60)"`
	RequireMessageSigning *bool          `option:"require-message-signing" help:"Mandates message signing otherwise does not allow the connection. If this is false, messaging signing is just enabled and not enforced. (default: false)"`
	Dialect               uint16         `option:"dialect" help:"Force a specific dialect to be used. SMB311:785, SMB302:770, SMB300:768, SMB210:528, SMB202:514, SMB2:767. If unspecfied (0), following dialects are tried in order - SMB311, SMB302, SMB300, SMB210, SMB202 (default: 0)"`
	ClientGuid            string         `option:"client-guid" help:"A 16-byte GUID to uniquely identify a client. If not specific a random GUID is used. (default: \"\")"`

	User     string               `option:"user"`
	Password options.SecretString `option:"password"`
	Domain   string               `option:"domain"`
}

const (
	DefaultSmbPort     int           = 445
	DefaultDomain      string        = "WORKGROUP"
	DefaultConnections uint          = 2
	DefaultIdleTimeout time.Duration = 60 * time.Second
)

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Port: DefaultSmbPort,
	}
}

func init() {
	options.Register("smb", Config{})
}

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are smb://address:port/sharename/directory and
// smb://address/sharename/directory in which case default port 445 is used.
// If no prefix is given the prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
	switch {
	case strings.HasPrefix(s, "smb://"):
		s = s[6:]
	case strings.HasPrefix(s, "smb:"):
		s = s[4:]
	default:
		return nil, errors.New("smb: invalid format")
	}
	// use the first entry of the path as the endpoint and the
	// remainder as bucket name and prefix
	fullAddress, rest, _ := strings.Cut(s, "/")
	address, portString, hasPort := strings.Cut(fullAddress, ":")
	var port int
	if !hasPort {
		port = DefaultSmbPort
	} else {
		var err error
		port, err = strconv.Atoi(portString)
		if err != nil {
			return nil, err
		}
	}
	sharename, directory, _ := strings.Cut(rest, "/")
	return createConfig(address, port, sharename, directory)
}

func createConfig(address string, port int, sharename string, directory string) (interface{}, error) {
	if address == "" {
		return nil, errors.New("smb: invalid format, address not found")
	}

	if directory != "" {
		directory = path.Clean(directory)
	}

	cfg := NewConfig()
	cfg.Address = address
	cfg.Port = port
	cfg.ShareName = sharename
	cfg.Path = directory
	return cfg, nil
}
