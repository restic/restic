package aws_test

import (
	"os"
	"strings"
	"testing"

	. "gopkg.in/check.v1"

	"gopkg.in/amz.v3/aws"
)

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&S{})

type S struct {
	environ []string
}

func (s *S) SetUpSuite(c *C) {
	s.environ = os.Environ()
}

func (s *S) TearDownTest(c *C) {
	os.Clearenv()
	for _, kv := range s.environ {
		l := strings.SplitN(kv, "=", 2)
		os.Setenv(l[0], l[1])
	}
}

func (s *S) TestEnvAuthNoSecret(c *C) {
	os.Clearenv()
	_, err := aws.EnvAuth()
	c.Assert(err, ErrorMatches, "AWS_SECRET_ACCESS_KEY not found in environment")
}

func (s *S) TestEnvAuthNoAccess(c *C) {
	os.Clearenv()
	os.Setenv("AWS_SECRET_ACCESS_KEY", "foo")
	_, err := aws.EnvAuth()
	c.Assert(err, ErrorMatches, "AWS_ACCESS_KEY_ID not found in environment")
}

func (s *S) TestEnvAuth(c *C) {
	os.Clearenv()
	os.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	os.Setenv("AWS_ACCESS_KEY_ID", "access")
	auth, err := aws.EnvAuth()
	c.Assert(err, IsNil)
	c.Assert(auth, Equals, aws.Auth{SecretKey: "secret", AccessKey: "access"})
}

func (s *S) TestEnvAuthLegacy(c *C) {
	os.Clearenv()
	os.Setenv("EC2_SECRET_KEY", "secret")
	os.Setenv("EC2_ACCESS_KEY", "access")
	auth, err := aws.EnvAuth()
	c.Assert(err, IsNil)
	c.Assert(auth, Equals, aws.Auth{SecretKey: "secret", AccessKey: "access"})
}

func (s *S) TestEnvAuthAws(c *C) {
	os.Clearenv()
	os.Setenv("AWS_SECRET_KEY", "secret")
	os.Setenv("AWS_ACCESS_KEY", "access")
	auth, err := aws.EnvAuth()
	c.Assert(err, IsNil)
	c.Assert(auth, Equals, aws.Auth{SecretKey: "secret", AccessKey: "access"})
}

func (s *S) TestEncode(c *C) {
	c.Assert(aws.Encode("foo"), Equals, "foo")
	c.Assert(aws.Encode("/"), Equals, "%2F")
}

func (s *S) TestRegionsAreNamed(c *C) {
	for n, r := range aws.Regions {
		c.Assert(n, Equals, r.Name)
	}
}
