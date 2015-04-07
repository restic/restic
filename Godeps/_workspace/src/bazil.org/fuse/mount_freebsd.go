package fuse

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func mount(dir string, conf *MountConfig, ready chan<- struct{}, errp *error) (*os.File, error) {
	for k, v := range conf.options {
		if strings.Contains(k, ",") || strings.Contains(v, ",") {
			// Silly limitation but the mount helper does not
			// understand any escaping. See TestMountOptionCommaError.
			return nil, fmt.Errorf("mount options cannot contain commas on FreeBSD: %q=%q", k, v)
		}
	}

	f, err := os.OpenFile("/dev/fuse", os.O_RDWR, 0000)
	if err != nil {
		*errp = err
		return nil, err
	}

	cmd := exec.Command(
		"/sbin/mount_fusefs",
		"--safe",
		"-o", conf.getOptions(),
		"3",
		dir,
	)
	cmd.ExtraFiles = []*os.File{f}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("mount_fusefs: %q, %v", out, err)
	}

	close(ready)
	return f, nil
}
