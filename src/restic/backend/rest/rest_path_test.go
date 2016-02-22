package rest

import (
	"net/url"
	"restic/backend"
	"testing"
)

var restPathTests = []struct {
	Handle backend.Handle
	URL    *url.URL
	Result string
}{
	{
		URL: parseURL("https://hostname.foo"),
		Handle: backend.Handle{
			Type: backend.Data,
			Name: "foobar",
		},
		Result: "https://hostname.foo/data/foobar",
	},
	{
		URL: parseURL("https://hostname.foo:1234/prefix/repo"),
		Handle: backend.Handle{
			Type: backend.Lock,
			Name: "foobar",
		},
		Result: "https://hostname.foo:1234/prefix/repo/locks/foobar",
	},
	{
		URL: parseURL("https://hostname.foo:1234/prefix/repo"),
		Handle: backend.Handle{
			Type: backend.Config,
			Name: "foobar",
		},
		Result: "https://hostname.foo:1234/prefix/repo/config",
	},
}

func TestRESTPaths(t *testing.T) {
	for i, test := range restPathTests {
		result := restPath(test.URL, test.Handle)
		if result != test.Result {
			t.Errorf("test %d: resulting URL does not match, want:\n  %#v\ngot: \n  %#v",
				i, test.Result, result)
		}
	}
}
