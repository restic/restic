package debug

import (
	"log"
	"os"
	"testing"
)

// TestLogToStderr configures debug to log to stderr if not the debug log is
// not already configured and returns whether logging was enabled.
func TestLogToStderr(t testing.TB) bool {
	if opts.isEnabled {
		return false
	}
	opts.logger = log.New(os.Stderr, "", log.LstdFlags)
	opts.isEnabled = true
	return true
}

func TestDisableLog(t testing.TB) {
	opts.logger = nil
	opts.isEnabled = false
}
