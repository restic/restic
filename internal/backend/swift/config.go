package swift

import (
	"os"
	"strings"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains basic configuration needed to specify swift location for a swift server
type Config struct {
	UserName       string
	UserID         string
	Domain         string
	DomainID       string
	APIKey         string
	AuthURL        string
	Region         string
	Tenant         string
	TenantID       string
	TenantDomain   string
	TenantDomainID string
	TrustID        string

	StorageURL string
	AuthToken  options.SecretString

	// auth v3 only
	ApplicationCredentialID     string
	ApplicationCredentialName   string
	ApplicationCredentialSecret options.SecretString

	Container              string
	Prefix                 string
	DefaultContainerPolicy string

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

func init() {
	options.Register("swift", Config{})
}

// NewConfig returns a new config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

// ParseConfig parses the string s and extract swift's container name and prefix.
func ParseConfig(s string) (*Config, error) {
	if !strings.HasPrefix(s, "swift:") {
		return nil, errors.New("invalid URL, expected: swift:container-name:/[prefix]")
	}
	s = strings.TrimPrefix(s, "swift:")

	container, prefix, _ := strings.Cut(s, ":")
	if prefix == "" {
		return nil, errors.Errorf("prefix is empty")
	}

	if prefix[0] != '/' {
		return nil, errors.Errorf("prefix does not start with slash (/)")
	}
	prefix = prefix[1:]

	cfg := NewConfig()
	cfg.Container = container
	cfg.Prefix = prefix

	return &cfg, nil
}

var _ backend.ApplyEnvironmenter = &Config{}

// ApplyEnvironment saves values from the environment to the config.
func (cfg *Config) ApplyEnvironment(prefix string) {
	for _, val := range []struct {
		s   *string
		env string
	}{
		// v2/v3 specific
		{&cfg.UserName, prefix + "OS_USERNAME"},
		{&cfg.APIKey, prefix + "OS_PASSWORD"},
		{&cfg.Region, prefix + "OS_REGION_NAME"},
		{&cfg.AuthURL, prefix + "OS_AUTH_URL"},

		// v3 specific
		{&cfg.UserID, prefix + "OS_USER_ID"},
		{&cfg.Domain, prefix + "OS_USER_DOMAIN_NAME"},
		{&cfg.DomainID, prefix + "OS_USER_DOMAIN_ID"},
		{&cfg.Tenant, prefix + "OS_PROJECT_NAME"},
		{&cfg.TenantDomain, prefix + "OS_PROJECT_DOMAIN_NAME"},
		{&cfg.TenantDomainID, prefix + "OS_PROJECT_DOMAIN_ID"},
		{&cfg.TrustID, prefix + "OS_TRUST_ID"},

		// v2 specific
		{&cfg.TenantID, prefix + "OS_TENANT_ID"},
		{&cfg.Tenant, prefix + "OS_TENANT_NAME"},

		// v1 specific
		{&cfg.AuthURL, prefix + "ST_AUTH"},
		{&cfg.UserName, prefix + "ST_USER"},
		{&cfg.APIKey, prefix + "ST_KEY"},

		// Application Credential auth
		{&cfg.ApplicationCredentialID, prefix + "OS_APPLICATION_CREDENTIAL_ID"},
		{&cfg.ApplicationCredentialName, prefix + "OS_APPLICATION_CREDENTIAL_NAME"},

		// Manual authentication
		{&cfg.StorageURL, prefix + "OS_STORAGE_URL"},

		{&cfg.DefaultContainerPolicy, prefix + "SWIFT_DEFAULT_CONTAINER_POLICY"},
	} {
		if *val.s == "" {
			*val.s = os.Getenv(val.env)
		}
	}
	for _, val := range []struct {
		s   *options.SecretString
		env string
	}{
		{&cfg.ApplicationCredentialSecret, prefix + "OS_APPLICATION_CREDENTIAL_SECRET"},
		{&cfg.AuthToken, prefix + "OS_AUTH_TOKEN"},
	} {
		if val.s.String() == "" {
			*val.s = options.NewSecretString(os.Getenv(val.env))
		}
	}
}
