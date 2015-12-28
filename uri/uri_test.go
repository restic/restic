package uri

import (
	"reflect"
	"testing"

	"github.com/restic/restic/backend/s3"
	"github.com/restic/restic/backend/sftp"
)

var parseTests = []struct {
	s string
	u URI
}{
	{"local:/srv/repo", URI{Scheme: "local", Config: "/srv/repo"}},
	{"local:dir1/dir2", URI{Scheme: "local", Config: "dir1/dir2"}},
	{"local:dir1/dir2", URI{Scheme: "local", Config: "dir1/dir2"}},
	{"dir1/dir2", URI{Scheme: "local", Config: "dir1/dir2"}},
	{"local:../dir1/dir2", URI{Scheme: "local", Config: "../dir1/dir2"}},
	{"/dir1/dir2", URI{Scheme: "local", Config: "/dir1/dir2"}},

	{"sftp:user@host:/srv/repo", URI{Scheme: "sftp",
		Config: sftp.Config{
			User: "user",
			Host: "host",
			Dir:  "/srv/repo",
		}}},
	{"sftp:host:/srv/repo", URI{Scheme: "sftp",
		Config: sftp.Config{
			User: "",
			Host: "host",
			Dir:  "/srv/repo",
		}}},
	{"sftp://user@host/srv/repo", URI{Scheme: "sftp",
		Config: sftp.Config{
			User: "user",
			Host: "host",
			Dir:  "srv/repo",
		}}},
	{"sftp://user@host//srv/repo", URI{Scheme: "sftp",
		Config: sftp.Config{
			User: "user",
			Host: "host",
			Dir:  "/srv/repo",
		}}},

	{"s3://eu-central-1/bucketname", URI{Scheme: "s3",
		Config: s3.Config{
			Host:   "eu-central-1",
			Bucket: "bucketname",
		}},
	},
	{"s3://hostname.foo/bucketname", URI{Scheme: "s3",
		Config: s3.Config{
			Host:   "hostname.foo",
			Bucket: "bucketname",
		}},
	},
	{"s3:hostname.foo:repo", URI{Scheme: "s3",
		Config: s3.Config{
			Host:   "hostname.foo",
			Bucket: "repo",
		}},
	},
}

func TestParseURI(t *testing.T) {
	for i, test := range parseTests {
		u, err := ParseURI(test.s)
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
