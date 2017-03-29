package swift

import (
	"net/url"
	"regexp"
	"restic/errors"
)

var (
	urlParser = regexp.MustCompile("^([^:]+):/(.*)$")
)

// Config contains basic configuration needed to specify swift location for a swift server
type Config struct {
	UserName     string
	Domain       string
	APIKey       string
	AuthURL      string
	Region       string
	Tenant       string
	TenantID     string
	TenantDomain string
	TrustID      string

	StorageURL string
	AuthToken  string

	Container              string
	Prefix                 string
	DefaultContainerPolicy string
}

// ParseConfig parses the string s and extract swift's container name and prefix.
func ParseConfig(s string) (interface{}, error) {

	url, err := url.Parse(s)
	if err != nil {
		return nil, errors.Wrap(err, "url.Parse")
	}

	m := urlParser.FindStringSubmatch(url.Opaque)
	if len(m) == 0 {
		return nil, errors.New("swift: invalid URL, valid syntax is: 'swift:container-name:/[optional-prefix]'")
	}

	cfg := Config{
		Container: m[1],
		Prefix:    m[2],
	}

	return cfg, nil
}
