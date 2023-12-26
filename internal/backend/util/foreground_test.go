//go:build !windows
// +build !windows

package util_test

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend/util"
	rtest "github.com/restic/restic/internal/test"
)

func TestForeground(t *testing.T) {
	err := os.Setenv("RESTIC_PASSWORD", "supersecret")
	rtest.OK(t, err)

	cmd := exec.Command("env")
	stdout, err := cmd.StdoutPipe()
	rtest.OK(t, err)

	bg, err := util.StartForeground(cmd)
	rtest.OK(t, err)
	defer func() {
		rtest.OK(t, cmd.Wait())
	}()

	err = bg()
	rtest.OK(t, err)

	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "RESTIC_PASSWORD=") {
			t.Error("subprocess got to see the password")
		}
	}
	rtest.OK(t, err)
}
