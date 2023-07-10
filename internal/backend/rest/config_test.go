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
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}
