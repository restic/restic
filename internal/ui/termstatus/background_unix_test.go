//go:build unix

package termstatus

import (
	"os"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestIsProcessBackground(t *testing.T) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		t.Skipf("can't open terminal: %v", err)
	}

	_, err = isProcessBackground(int(tty.Fd()))
	rtest.OK(t, err)

	_ = tty.Close()
}
