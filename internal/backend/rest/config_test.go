package rest

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/backend/test"
	rtest "github.com/restic/restic/internal/test"
)

func parseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}

	return u
}

var configTests = []test.ConfigTestData[Config]{
	{
		S: "rest:http://localhost:1234",
		Cfg: Config{
			URL:         parseURL("http://localhost:1234/"),
			Connections: 5,
		},
	},
	{
		S: "rest:http://localhost:1234/",
		Cfg: Config{
			URL:         parseURL("http://localhost:1234/"),
			Connections: 5,
		},
	},
	{
		S: "rest:http+unix:///tmp/rest.socket:/my_backup_repo/",
		Cfg: Config{
			URL:         parseURL("http+unix:///tmp/rest.socket:/my_backup_repo/"),
			Connections: 5,
		},
	},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}

var passwordTests = []struct {
	input    string
	expected string
}{
	{
		"rest:",
		"rest:/",
	},
	{
		"rest:localhost/",
		"rest:localhost/",
	},
	{
		"rest::123/",
		"rest::123/",
	},
	{
		"rest:http://",
		"rest:http://",
	},
	{
		"rest:http://hostname.foo:1234/",
		"rest:http://hostname.foo:1234/",
	},
	{
		"rest:http://user@hostname.foo:1234/",
		"rest:http://user@hostname.foo:1234/",
	},
	{
		"rest:http://user:@hostname.foo:1234/",
		"rest:http://user:***@hostname.foo:1234/",
	},
	{
		"rest:http://user:p@hostname.foo:1234/",
		"rest:http://user:***@hostname.foo:1234/",
	},
	{
		"rest:http://user:pppppaaafhhfuuwiiehhthhghhdkjaoowpprooghjjjdhhwuuhgjsjhhfdjhruuhsjsdhhfhshhsppwufhhsjjsjs@hostname.foo:1234/",
		"rest:http://user:***@hostname.foo:1234/",
	},
	{
		"rest:http://user:password@hostname",
		"rest:http://user:***@hostname/",
	},
	{
		"rest:http://user:password@:123",
		"rest:http://user:***@:123/",
	},
	{
		"rest:http://user:password@",
		"rest:http://user:***@/",
	},
}

func TestStripPassword(t *testing.T) {
	// Make sure that the factory uses the correct method
	StripPassword := NewFactory().StripPassword

	for i, test := range passwordTests {
		t.Run(test.input, func(t *testing.T) {
			result := StripPassword(test.input)
			if result != test.expected {
				t.Errorf("test %d: expected '%s' but got '%s'", i, test.expected, result)
			}
		})
	}
}

func TestApplyEnvironmentPasswordFile(t *testing.T) {
	cfg, err := ParseConfig("rest:http://localhost:1234/")
	rtest.OK(t, err)

	t.Setenv("RESTIC_REST_PASSWORD_FILE", "/some/path/password")
	cfg.ApplyEnvironment("")

	rtest.Equals(t, "/some/path/password", cfg.PasswordFile)
}

func TestOpenPasswordFile(t *testing.T) {
	dir := t.TempDir()
	pwdFile := filepath.Join(dir, "password")
	rtest.OK(t, os.WriteFile(pwdFile, []byte("secret\n"), 0600))

	cfg, err := ParseConfig("rest:http://localhost:1234/")
	rtest.OK(t, err)
	cfg.PasswordFile = pwdFile

	be, err := Open(t.Context(), *cfg, http.DefaultTransport, nil)
	rtest.OK(t, err)

	pwd, set := be.url.User.Password()
	rtest.Assert(t, set, "expected password to be set")
	rtest.Equals(t, "secret", pwd)
}

func TestOpenPasswordFileMissing(t *testing.T) {
	cfg, err := ParseConfig("rest:http://localhost:1234/")
	rtest.OK(t, err)
	cfg.PasswordFile = "/nonexistent/path/password"

	_, err = Open(t.Context(), *cfg, http.DefaultTransport, nil)
	rtest.Assert(t, err != nil, "expected error for missing password file")
}

func TestOpenPasswordFilePreferredOverPassword(t *testing.T) {
	dir := t.TempDir()
	pwdFile := filepath.Join(dir, "password")
	rtest.OK(t, os.WriteFile(pwdFile, []byte("from-file\n"), 0600))

	cfg, err := ParseConfig("rest:http://localhost:1234/")
	rtest.OK(t, err)

	// Simulate both env vars being set: ApplyEnvironment stores the direct password
	// in the URL and the file path in PasswordFile. Open should prefer the file.
	cfg.URL.User = url.UserPassword("", "from-env")
	cfg.PasswordFile = pwdFile

	be, err := Open(t.Context(), *cfg, http.DefaultTransport, nil)
	rtest.OK(t, err)

	pwd, set := be.url.User.Password()
	rtest.Assert(t, set, "expected password to be set")
	rtest.Equals(t, "from-file", pwd)
}
