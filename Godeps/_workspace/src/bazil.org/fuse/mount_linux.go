package fuse

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

func lineLogger(wg *sync.WaitGroup, prefix string, r io.ReadCloser) {
	defer wg.Done()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		switch line := scanner.Text(); line {
		case `fusermount: failed to open /etc/fuse.conf: Permission denied`:
			// Silence this particular message, it occurs way too
			// commonly and isn't very relevant to whether the mount
			// succeeds or not.
			continue
		default:
			log.Printf("%s: %s", prefix, line)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("%s, error reading: %v", prefix, err)
	}
}

func mount(dir string, conf *mountConfig, ready chan<- struct{}, errp *error) (fusefd *os.File, err error) {
	// linux mount is never delayed
	close(ready)

	fds, err := syscall.Socketpair(syscall.AF_FILE, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("socketpair error: %v", err)
	}

	writeFile := os.NewFile(uintptr(fds[0]), "fusermount-child-writes")
	defer writeFile.Close()

	readFile := os.NewFile(uintptr(fds[1]), "fusermount-parent-reads")
	defer readFile.Close()

	cmd := exec.Command(
		"fusermount",
		"-o", conf.getOptions(),
		"--",
		dir,
	)
	cmd.Env = append(os.Environ(), "_FUSE_COMMFD=3")

	cmd.ExtraFiles = []*os.File{writeFile}

	var wg sync.WaitGroup
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("setting up fusermount stderr: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("setting up fusermount stderr: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("fusermount: %v", err)
	}
	wg.Add(2)
	go lineLogger(&wg, "mount helper output", stdout)
	go lineLogger(&wg, "mount helper error", stderr)
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("fusermount: %v", err)
	}

	c, err := net.FileConn(readFile)
	if err != nil {
		return nil, fmt.Errorf("FileConn from fusermount socket: %v", err)
	}
	defer c.Close()

	uc, ok := c.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("unexpected FileConn type; expected UnixConn, got %T", c)
	}

	buf := make([]byte, 32) // expect 1 byte
	oob := make([]byte, 32) // expect 24 bytes
	_, oobn, _, _, err := uc.ReadMsgUnix(buf, oob)
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, fmt.Errorf("ParseSocketControlMessage: %v", err)
	}
	if len(scms) != 1 {
		return nil, fmt.Errorf("expected 1 SocketControlMessage; got scms = %#v", scms)
	}
	scm := scms[0]
	gotFds, err := syscall.ParseUnixRights(&scm)
	if err != nil {
		return nil, fmt.Errorf("syscall.ParseUnixRights: %v", err)
	}
	if len(gotFds) != 1 {
		return nil, fmt.Errorf("wanted 1 fd; got %#v", gotFds)
	}
	f := os.NewFile(uintptr(gotFds[0]), "/dev/fuse")
	return f, nil
}
