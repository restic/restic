package location

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/restic/restic/internal/backend/b2"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/rest"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/backend/swift"
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
	{
		"local:/srv/repo",
		Location{Scheme: "local",
			Config: local.Config{
				Path:        "/srv/repo",
				Connections: 2,
			},
		},
	},
	{
		"local:dir1/dir2",
		Location{Scheme: "local",
			Config: local.Config{
				Path:        "dir1/dir2",
				Connections: 2,
			},
		},
	},
	{
		"local:dir1/dir2",
		Location{Scheme: "local",
			Config: local.Config{
				Path:        "dir1/dir2",
				Connections: 2,
			},
		},
	},
	{
		"dir1/dir2",
		Location{Scheme: "local",
			Config: local.Config{
				Path:        "dir1/dir2",
				Connections: 2,
			},
		},
	},
	{
		"/dir1/dir2",
		Location{Scheme: "local",
			Config: local.Config{
				Path:        "/dir1/dir2",
				Connections: 2,
			},
		},
	},
	{
		"local:../dir1/dir2",
		Location{Scheme: "local",
			Config: local.Config{
				Path:        "../dir1/dir2",
				Connections: 2,
			},
		},
	},
	{
		"/dir1/dir2",
		Location{Scheme: "local",
			Config: local.Config{
				Path:        "/dir1/dir2",
				Connections: 2,
			},
		},
	},
	{
		"/dir1:foobar/dir2",
		Location{Scheme: "local",
			Config: local.Config{
				Path:        "/dir1:foobar/dir2",
				Connections: 2,
			},
		},
	},
	{
		`\dir1\foobar\dir2`,
		Location{Scheme: "local",
			Config: local.Config{
				Path:        `\dir1\foobar\dir2`,
				Connections: 2,
			},
		},
	},
	{
		`c:\dir1\foobar\dir2`,
		Location{Scheme: "local",
			Config: local.Config{
				Path:        `c:\dir1\foobar\dir2`,
				Connections: 2,
			},
		},
	},
	{
		`C:\Users\appveyor\AppData\Local\Temp\1\restic-test-879453535\repo`,
		Location{Scheme: "local",
			Config: local.Config{
				Path:        `C:\Users\appveyor\AppData\Local\Temp\1\restic-test-879453535\repo`,
				Connections: 2,
			},
		},
	},
	{
		`c:/dir1/foobar/dir2`,
		Location{Scheme: "local",
			Config: local.Config{
				Path:        `c:/dir1/foobar/dir2`,
				Connections: 2,
			},
		},
	},
	{
		"sftp:user@host:/srv/repo",
		Location{Scheme: "sftp",
			Config: sftp.Config{
				User:        "user",
				Host:        "host",
				Path:        "/srv/repo",
				Connections: 5,
			},
		},
	},
	{
		"sftp:host:/srv/repo",
		Location{Scheme: "sftp",
			Config: sftp.Config{
				User:        "",
				Host:        "host",
				Path:        "/srv/repo",
				Connections: 5,
			},
		},
	},
	{
		"sftp://user@host/srv/repo",
		Location{Scheme: "sftp",
			Config: sftp.Config{
				User:        "user",
				Host:        "host",
				Path:        "srv/repo",
				Connections: 5,
			},
		},
	},
	{
		"sftp://user@host//srv/repo",
		Location{Scheme: "sftp",
			Config: sftp.Config{
				User:        "user",
				Host:        "host",
				Path:        "/srv/repo",
				Connections: 5,
			},
		},
	},

	{
		"s3://eu-central-1/bucketname",
		Location{Scheme: "s3",
			Config: s3.Config{
				Endpoint:    "eu-central-1",
				Bucket:      "bucketname",
				Prefix:      "",
				Connections: 5,
			},
		},
	},
	{
		"s3://hostname.foo/bucketname",
		Location{Scheme: "s3",
			Config: s3.Config{
				Endpoint:    "hostname.foo",
				Bucket:      "bucketname",
				Prefix:      "",
				Connections: 5,
			},
		},
	},
	{
		"s3://hostname.foo/bucketname/prefix/directory",
		Location{Scheme: "s3",
			Config: s3.Config{
				Endpoint:    "hostname.foo",
				Bucket:      "bucketname",
				Prefix:      "prefix/directory",
				Connections: 5,
			},
		},
	},
	{
		"s3:eu-central-1/repo",
		Location{Scheme: "s3",
			Config: s3.Config{
				Endpoint:    "eu-central-1",
				Bucket:      "repo",
				Prefix:      "",
				Connections: 5,
			},
		},
	},
	{
		"s3:eu-central-1/repo/prefix/directory",
		Location{Scheme: "s3",
			Config: s3.Config{
				Endpoint:    "eu-central-1",
				Bucket:      "repo",
				Prefix:      "prefix/directory",
				Connections: 5,
			},
		},
	},
	{
		"s3:https://hostname.foo/repo",
		Location{Scheme: "s3",
			Config: s3.Config{
				Endpoint:    "hostname.foo",
				Bucket:      "repo",
				Prefix:      "",
				Connections: 5,
			},
		},
	},
	{
		"s3:https://hostname.foo/repo/prefix/directory",
		Location{Scheme: "s3",
			Config: s3.Config{
				Endpoint:    "hostname.foo",
				Bucket:      "repo",
				Prefix:      "prefix/directory",
				Connections: 5,
			},
		},
	},
	{
		"s3:http://hostname.foo/repo",
		Location{Scheme: "s3",
			Config: s3.Config{
				Endpoint:    "hostname.foo",
				Bucket:      "repo",
				Prefix:      "",
				UseHTTP:     true,
				Connections: 5,
			},
		},
	},
	{
		"swift:container17:/",
		Location{Scheme: "swift",
			Config: swift.Config{
				Container:   "container17",
				Prefix:      "",
				Connections: 5,
			},
		},
	},
	{
		"swift:container17:/prefix97",
		Location{Scheme: "swift",
			Config: swift.Config{
				Container:   "container17",
				Prefix:      "prefix97",
				Connections: 5,
			},
		},
	},
	{
		"rest:http://hostname.foo:1234/",
		Location{Scheme: "rest",
			Config: rest.Config{
				URL:         parseURL("http://hostname.foo:1234/"),
				Connections: 5,
			},
		},
	},
	{
		"b2:bucketname:/prefix", Location{Scheme: "b2",
			Config: b2.Config{
				Bucket:      "bucketname",
				Prefix:      "prefix",
				Connections: 5,
			},
		},
	},
	{
		"b2:bucketname", Location{Scheme: "b2",
			Config: b2.Config{
				Bucket:      "bucketname",
				Prefix:      "",
				Connections: 5,
			},
		},
	},
}

func TestParse(t *testing.T) {
	for i, test := range parseTests {
		t.Run(test.s, func(t *testing.T) {
			u, err := Parse(test.s)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if test.u.Scheme != u.Scheme {
				t.Errorf("test %d: scheme does not match, want %q, got %q",
					i, test.u.Scheme, u.Scheme)
			}

			if !reflect.DeepEqual(test.u.Config, u.Config) {
				t.Errorf("test %d: cfg map does not match, want:\n  %#v\ngot: \n  %#v",
					i, test.u.Config, u.Config)
			}
		})
	}
}

func TestInvalidScheme(t *testing.T) {
	var invalidSchemes = []string{
		"foobar:xxx",
		"foobar:/dir/dir2",
	}

	for _, s := range invalidSchemes {
		t.Run(s, func(t *testing.T) {
			_, err := Parse(s)
			if err == nil {
				t.Fatalf("error for invalid location %q not found", s)
			}
		})
	}
}
