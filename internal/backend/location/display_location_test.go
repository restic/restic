package location

import "testing"

var passwordTests = []struct {
	input    string
	expected string
}{
	{
		"local:/srv/repo",
		"local:/srv/repo",
	},
	{
		"/dir1/dir2",
		"/dir1/dir2",
	},
	{
		`c:\dir1\foobar\dir2`,
		`c:\dir1\foobar\dir2`,
	},
	{
		"sftp:user@host:/srv/repo",
		"sftp:user@host:/srv/repo",
	},
	{
		"s3://eu-central-1/bucketname",
		"s3://eu-central-1/bucketname",
	},
	{
		"swift:container17:/prefix97",
		"swift:container17:/prefix97",
	},
	{
		"b2:bucketname:/prefix",
		"b2:bucketname:/prefix",
	},
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
	for i, test := range passwordTests {
		t.Run(test.input, func(t *testing.T) {
			result := StripPassword(test.input)
			if result != test.expected {
				t.Errorf("test %d: expected '%s' but got '%s'", i, test.expected, result)
			}
		})
	}
}
