package test

import (
	"fmt"
	"os"
)

var (
	TestPassword          = getStringVar("RESTIC_TEST_PASSWORD", "geheim")
	TestCleanupTempDirs   = getBoolVar("RESTIC_TEST_CLEANUP", true)
	TestTempDir           = getStringVar("RESTIC_TEST_TMPDIR", "")
	RunIntegrationTest    = getBoolVar("RESTIC_TEST_INTEGRATION", true)
	RunFuseTest           = getBoolVar("RESTIC_TEST_FUSE", true)
	TestSFTPPath          = getStringVar("RESTIC_TEST_SFTPPATH", "/usr/lib/ssh:/usr/lib/openssh")
	TestWalkerPath        = getStringVar("RESTIC_TEST_PATH", ".")
	BenchArchiveDirectory = getStringVar("RESTIC_BENCH_DIR", ".")
	TestS3Server          = getStringVar("RESTIC_TEST_S3_SERVER", "")
	TestRESTServer        = getStringVar("RESTIC_TEST_REST_SERVER", "")
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
		case "1":
			return true
		case "0":
			return false
		default:
			fmt.Fprintf(os.Stderr, "invalid value for variable %q, using default\n", name)
		}
	}

	return defaultValue
}
