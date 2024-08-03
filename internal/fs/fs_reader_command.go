package fs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/restic/restic/internal/errors"
)

// CommandReader wrap a command such that its standard output can be read using
// a io.ReadCloser. Close() waits for the command to terminate, reporting
// any error back to the caller.
type CommandReader struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser

	// cmd.Wait() must only be called once. Prevent duplicate executions in
	// Read() and Close().
	waitHandled bool

	// alreadyClosedReadErr is the error that we should return if we try to
	// read the pipe again after closing. This works around a Read() call that
	// is issued after a previous Read() with `io.EOF` (but some bytes were
	// read in the past).
	alreadyClosedReadErr error
}

func NewCommandReader(ctx context.Context, args []string, logOutput io.Writer) (*CommandReader, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no command was specified as argument")
	}

	// Prepare command and stdout
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to setup stdout pipe: %w", err)
	}

	// Use a Go routine to handle the stderr to avoid deadlocks
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to setup stderr pipe: %w", err)
	}
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			_, _ = fmt.Fprintf(logOutput, "subprocess %v: %v\n", command.Args[0], sc.Text())
		}
	}()

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	return &CommandReader{
		cmd:    command,
		stdout: stdout,
	}, nil
}

// Read populate the array with data from the process stdout.
func (fp *CommandReader) Read(p []byte) (int, error) {
	if fp.alreadyClosedReadErr != nil {
		return 0, fp.alreadyClosedReadErr
	}
	b, err := fp.stdout.Read(p)

	// If the error is io.EOF, the program terminated. We need to check the
	// exit code here because, if the program terminated with no output, the
	// error in `Close()` is ignored.
	if errors.Is(err, io.EOF) {
		fp.waitHandled = true
		// check if the command terminated successfully, If not return the error.
		if errw := fp.wait(); errw != nil {
			err = errw
		}
	}
	fp.alreadyClosedReadErr = err
	return b, err
}

func (fp *CommandReader) wait() error {
	err := fp.cmd.Wait()
	if err != nil {
		// Use a fatal error to abort the snapshot.
		return errors.Fatal(fmt.Errorf("command failed: %w", err).Error())
	}
	return nil
}

func (fp *CommandReader) Close() error {
	if fp.waitHandled {
		return nil
	}

	return fp.wait()
}
