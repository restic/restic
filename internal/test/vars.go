package test

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

var (
	TestPassword                = getStringVar("RESTIC_TEST_PASSWORD", "geheim")
	TestCleanupTempDirs         = getBoolVar("RESTIC_TEST_CLEANUP", true)
	TestTempDir                 = getStringVar("RESTIC_TEST_TMPDIR", "")
	RunIntegrationTest          = getBoolVar("RESTIC_TEST_INTEGRATION", true)
	RunFuseTest                 = getBoolVar("RESTIC_TEST_FUSE", true)
	TestSFTPPath                = getStringVar("RESTIC_TEST_SFTPPATH", "/usr/lib/ssh:/usr/lib/openssh:/usr/libexec")
	TestWalkerPath              = getStringVar("RESTIC_TEST_PATH", ".")
	BenchArchiveDirectory       = getStringVar("RESTIC_BENCH_DIR", ".")
	TestS3Server                = getStringVar("RESTIC_TEST_S3_SERVER", "")
	TestRESTServer              = getStringVar("RESTIC_TEST_REST_SERVER", "")
	TestIntegrationDisallowSkip = getStringVar("RESTIC_TEST_DISALLOW_SKIP", "")
)

func getStringVar(name, defaultValue string) string {
	if e := os.Getenv(name); e != "" {
		return e
	}

	return defaultValue
}

func getBoolVar(name string, defaultValue bool) bool {
	if e := os.Getenv(name); e != "" {
		switch e {
		case "1", "true":
			return true
		case "0", "false":
			return false
		default:
			fmt.Fprintf(os.Stderr, "invalid value for variable %q, using default\n", name)
		}
	}

	return defaultValue
}

// SkipDisallowed fails the test if it needs to run. The environment
// variable RESTIC_TEST_DISALLOW_SKIP contains a comma-separated list of test
// names that must be run. If name is in this list, the test is marked as
// failed.
func SkipDisallowed(t testing.TB, name string) {
	for _, s := range strings.Split(TestIntegrationDisallowSkip, ",") {
		if s == name {
			t.Fatalf("test %v is in list of tests that need to run ($RESTIC_TEST_DISALLOW_SKIP)", name)
		}
	}
}
