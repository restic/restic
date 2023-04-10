package sftp

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	//"github.com/cenkalti/backoff/v4"
	"github.com/pkg/sftp"
)

type Connection struct {
	c           *sftp.Client
	cmd         *exec.Cmd
	posixRename bool
	result      <-chan error
}

func NewConnection(cfg Config) (*Connection, error) {
	program, args, err := buildSSHCommand(cfg)
	if err != nil {
		return nil, err
	}

	debug.Log("start client %v %v", program, args)
	// Connect to a remote host and request the sftp subsystem via the 'ssh'
	// command.  This assumes that passwordless login is correctly configured.
	cmd := exec.Command(program, args...)

	// prefix the errors with the program name
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.StderrPipe")
	}

	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			fmt.Fprintf(os.Stderr, "subprocess %v: %v\n", program, sc.Text())
		}
	}()

	// get stdin and stdout
	wr, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.StdinPipe")
	}
	rd, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.StdoutPipe")
	}

	bg, err := backend.StartForeground(cmd)
	if err != nil {
		if backend.IsErrDot(err) {
			return nil, errors.Errorf("cannot implicitly run relative executable %v found in current directory, use -o sftp.command=./<command> to override", cmd.Path)
		}
		return nil, err
	}

	// wait in a different goroutine
	ch := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		// TODO(ibash) try to reconnect here... or in client error do the thing
		debug.Log("ssh command exited, err %v", err)
		for {
			ch <- errors.Wrap(err, "ssh command exited")
		}
	}()

	// open the SFTP session
	client, err := sftp.NewClientPipe(rd, wr)
	if err != nil {
		return nil, errors.Errorf("unable to start the sftp session, error: %v", err)
	}

	err = bg()
	if err != nil {
		return nil, errors.Wrap(err, "bg")
	}

	_, posixRename := client.HasExtension("posix-rename@openssh.com")
	return &Connection{c: client, cmd: cmd, result: ch, posixRename: posixRename}, nil
}

var closeTimeout = 2 * time.Second

// Close closes the sftp connection and terminates the underlying command.
func (c *Connection) Close() error {
	if c == nil {
		return nil
	}

	err := c.c.Close()
	debug.Log("Close returned error %v", err)

	// wait for closeTimeout before killing the process
	select {
	case err := <-c.result:
		return err
	case <-time.After(closeTimeout):
	}

	if err := c.cmd.Process.Kill(); err != nil {
		return err
	}

	// get the error, but ignore it
	<-c.result
	return nil
}

func (c *Connection) clientError() error {
	select {
	case err := <-c.result:
		debug.Log("client has exited with err %v", err)
		return err
	default:
	}

	return nil
}
