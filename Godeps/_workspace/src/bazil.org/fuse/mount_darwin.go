package fuse

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

var errNoAvail = errors.New("no available fuse devices")

var errNotLoaded = errors.New("osxfusefs is not loaded")

func loadOSXFUSE() error {
	cmd := exec.Command("/Library/Filesystems/osxfusefs.fs/Support/load_osxfusefs")
	cmd.Dir = "/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return err
}

func openOSXFUSEDev() (*os.File, error) {
	var f *os.File
	var err error
	for i := uint64(0); ; i++ {
		path := "/dev/osxfuse" + strconv.FormatUint(i, 10)
		f, err = os.OpenFile(path, os.O_RDWR, 0000)
		if os.IsNotExist(err) {
			if i == 0 {
				// not even the first device was found -> fuse is not loaded
				return nil, errNotLoaded
			}

			// we've run out of kernel-provided devices
			return nil, errNoAvail
		}

		if err2, ok := err.(*os.PathError); ok && err2.Err == syscall.EBUSY {
			// try the next one
			continue
		}

		if err != nil {
			return nil, err
		}
		return f, nil
	}
}

func callMount(dir string, conf *MountConfig, f *os.File, ready chan<- struct{}, errp *error) error {
	bin := "/Library/Filesystems/osxfusefs.fs/Support/mount_osxfusefs"

	for k, v := range conf.options {
		if strings.Contains(k, ",") || strings.Contains(v, ",") {
			// Silly limitation but the mount helper does not
			// understand any escaping. See TestMountOptionCommaError.
			return fmt.Errorf("mount options cannot contain commas on darwin: %q=%q", k, v)
		}
	}
	cmd := exec.Command(
		bin,
		"-o", conf.getOptions(),
		// Tell osxfuse-kext how large our buffer is. It must split
		// writes larger than this into multiple writes.
		//
		// OSXFUSE seems to ignore InitResponse.MaxWrite, and uses
		// this instead.
		"-o", "iosize="+strconv.FormatUint(maxWrite, 10),
		// refers to fd passed in cmd.ExtraFiles
		"3",
		dir,
	)
	cmd.ExtraFiles = []*os.File{f}
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "MOUNT_FUSEFS_CALL_BY_LIB=")
	// TODO this is used for fs typenames etc, let app influence it
	cmd.Env = append(cmd.Env, "MOUNT_FUSEFS_DAEMON_PATH="+bin)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Start()
	if err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		if err != nil {
			if buf.Len() > 0 {
				output := buf.Bytes()
				output = bytes.TrimRight(output, "\n")
				msg := err.Error() + ": " + string(output)
				err = errors.New(msg)
			}
		}
		*errp = err
		close(ready)
	}()
	return err
}

func mount(dir string, conf *MountConfig, ready chan<- struct{}, errp *error) (*os.File, error) {
	f, err := openOSXFUSEDev()
	if err == errNotLoaded {
		err = loadOSXFUSE()
		if err != nil {
			return nil, err
		}
		// try again
		f, err = openOSXFUSEDev()
	}
	if err != nil {
		return nil, err
	}
	err = callMount(dir, conf, f, ready, errp)
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
