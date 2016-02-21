package location

import (
	"net/url"
	"reflect"
	"testing"

	"restic/backend/rest"
	"restic/backend/s3"
	"restic/backend/sftp"
)

func parseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}

	return u
}

var parseTests = []struct {
	s string
	u Location
}{
	{"local:/srv/repo", Location{Scheme: "local", Config: "/srv/repo"}},
	{"local:dir1/dir2", Location{Scheme: "local", Config: "dir1/dir2"}},
	{"local:dir1/dir2", Location{Scheme: "local", Config: "dir1/dir2"}},
	{"dir1/dir2", Location{Scheme: "local", Config: "dir1/dir2"}},
	{"local:../dir1/dir2", Location{Scheme: "local", Config: "../dir1/dir2"}},
	{"/dir1/dir2", Location{Scheme: "local", Config: "/dir1/dir2"}},

	{"sftp:user@host:/srv/repo", Location{Scheme: "sftp",
		Config: sftp.Config{
			User: "user",
			Host: "host",
			Dir:  "/srv/repo",
		}}},
	{"sftp:host:/srv/repo", Location{Scheme: "sftp",
		Config: sftp.Config{
			User: "",
			Host: "host",
			Dir:  "/srv/repo",
		}}},
	{"sftp://user@host/srv/repo", Location{Scheme: "sftp",
		Config: sftp.Config{
			User: "user",
			Host: "host",
			Dir:  "srv/repo",
		}}},
	{"sftp://user@host//srv/repo", Location{Scheme: "sftp",
		Config: sftp.Config{
			User: "user",
			Host: "host",
			Dir:  "/srv/repo",
		}}},

	{"s3://eu-central-1/bucketname", Location{Scheme: "s3",
		Config: s3.Config{
			Endpoint: "eu-central-1",
			Bucket:   "bucketname",
			Prefix:   "restic",
		}},
	},
	{"s3://hostname.foo/bucketname", Location{Scheme: "s3",
		Config: s3.Config{
			Endpoint: "hostname.foo",
			Bucket:   "bucketname",
			Prefix:   "restic",
		}},
	},
	{"s3://hostname.foo/bucketname/prefix/directory", Location{Scheme: "s3",
		Config: s3.Config{
			Endpoint: "hostname.foo",
			Bucket:   "bucketname",
			Prefix:   "prefix/directory",
		}},
	},
	{"s3:eu-central-1/repo", Location{Scheme: "s3",
		Config: s3.Config{
			Endpoint: "eu-central-1",
			Bucket:   "repo",
			Prefix:   "restic",
		}},
	},
	{"s3:eu-central-1/repo/prefix/directory", Location{Scheme: "s3",
		Config: s3.Config{
			Endpoint: "eu-central-1",
			Bucket:   "repo",
			Prefix:   "prefix/directory",
		}},
	},
	{"s3:https://hostname.foo/repo", Location{Scheme: "s3",
		Config: s3.Config{
			Endpoint: "hostname.foo",
			Bucket:   "repo",
			Prefix:   "restic",
		}},
	},
	{"s3:https://hostname.foo/repo/prefix/directory", Location{Scheme: "s3",
		Config: s3.Config{
			Endpoint: "hostname.foo",
			Bucket:   "repo",
			Prefix:   "prefix/directory",
		}},
	},
	{"s3:http://hostname.foo/repo", Location{Scheme: "s3",
		Config: s3.Config{
			Endpoint: "hostname.foo",
			Bucket:   "repo",
			Prefix:   "restic",
			UseHTTP:  true,
		}},
	},
	{"rest:http://hostname.foo:1234/", Location{Scheme: "rest",
		Config: rest.Config{
			URL: parseURL("http://hostname.foo:1234/"),
		}},
	},
}

func TestParse(t *testing.T) {
	for i, test := range parseTests {
		u, err := Parse(test.s)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}

		if test.u.Scheme != u.Scheme {
			t.Errorf("test %d: scheme does not match, want %q, got %q",
				i, test.u.Scheme, u.Scheme)
		}

		if !reflect.DeepEqual(test.u.Config, u.Config) {
			t.Errorf("test %d: cfg map does not match, want:\n  %#v\ngot: \n  %#v",
				i, test.u.Config, u.Config)
		}
	}
}
