package location

import (
	"reflect"
	"testing"

	"github.com/restic/restic/backend/gcs"
	"github.com/restic/restic/backend/s3"
	"github.com/restic/restic/backend/sftp"
)

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

	{"gs://bucketname", Location{Scheme: "gs",
		Config: gcs.Config{
			Endpoint: "storage.googleapis.com",
			Bucket:   "bucketname",
			Prefix:   "restic",
		}},
	},
	{"gs://bucketname/prefix/directory", Location{Scheme: "gs",
		Config: gcs.Config{
			Endpoint: "storage.googleapis.com",
			Bucket:   "bucketname",
			Prefix:   "prefix/directory",
		}},
	},
	{"gs:bucketname", Location{Scheme: "gs",
		Config: gcs.Config{
			Endpoint: "storage.googleapis.com",
			Bucket:   "bucketname",
			Prefix:   "restic",
		}},
	},
	{"gs:bucketname/prefix/directory", Location{Scheme: "gs",
		Config: gcs.Config{
			Endpoint: "storage.googleapis.com",
			Bucket:   "bucketname",
			Prefix:   "prefix/directory",
		}},
	},
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
