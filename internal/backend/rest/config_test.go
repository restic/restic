package rest

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
