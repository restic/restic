package webdav

import (
	"net/url"
	"testing"

	"github.com/restic/restic/internal/backend/test"
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
		S: "webdav:http://localhost:1234",
		Cfg: Config{
			URL:         parseURL("http://localhost:1234/"),
			Connections: 5,
		},
	},
	{
		S: "webdav:http://localhost:1234/",
		Cfg: Config{
			URL:         parseURL("http://localhost:1234/"),
			Connections: 5,
		},
	},
	{
		S: "webdav:http+unix:///tmp/webdav.socket:/my_backup_repo/",
		Cfg: Config{
			URL:         parseURL("http+unix:///tmp/webdav.socket:/my_backup_repo/"),
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
		"webdav:",
		"webdav:/",
	},
	{
		"webdav:localhost/",
		"webdav:localhost/",
	},
	{
		"webdav::123/",
		"webdav::123/",
	},
	{
		"webdav:http://",
		"webdav:http://",
	},
	{
		"webdav:http://hostname.foo:1234/",
		"webdav:http://hostname.foo:1234/",
	},
	{
		"webdav:http://user@hostname.foo:1234/",
		"webdav:http://user@hostname.foo:1234/",
	},
	{
		"webdav:http://user:@hostname.foo:1234/",
		"webdav:http://user:***@hostname.foo:1234/",
	},
	{
		"webdav:http://user:p@hostname.foo:1234/",
		"webdav:http://user:***@hostname.foo:1234/",
	},
	{
		"webdav:http://user:pppppaaafhhfuuwiiehhthhghhdkjaoowpprooghjjjdhhwuuhgjsjhhfdjhruuhsjsdhhfhshhsppwufhhsjjsjs@hostname.foo:1234/",
		"webdav:http://user:***@hostname.foo:1234/",
	},
	{
		"webdav:http://user:password@hostname",
		"webdav:http://user:***@hostname/",
	},
	{
		"webdav:http://user:password@:123",
		"webdav:http://user:***@:123/",
	},
	{
		"webdav:http://user:password@",
		"webdav:http://user:***@/",
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
