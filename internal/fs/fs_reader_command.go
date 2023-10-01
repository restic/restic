package fs

import (
	"io"
	"os/exec"

	"github.com/restic/restic/internal/errors"
)

// CommandReader wraps an exec.Cmd and its standard output to provide an
// io.ReadCloser that waits for the command to terminate on Close(), reporting
// any error in the command.Wait() function back to the Close() caller.
type CommandReader struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser

	// We should call exec.Wait() once. waitHandled is taking care of storing
	// whether we already called that function in Read() to avoid calling it
	// again in Close().
	waitHandled bool

	// alreadyClosedReadErr is the error that we should return if we try to
	// read the pipe again after closing. This works around a Read() call that
	// is issued after a previous Read() with `io.EOF` (but some bytes were
	// read in the past).
	alreadyClosedReadErr error
}

func NewCommandReader(cmd *exec.Cmd, stdout io.ReadCloser) *CommandReader {
	return &CommandReader{
		cmd:    cmd,
		stdout: stdout,
	}
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
		// If we have information about the exit code, let's use it in the
		// error message. Otherwise, send the error message along.
		// In any case, use a fatal error to abort the snapshot.
		var err2 *exec.ExitError
		if errors.As(err, &err2) {
			return errors.Fatalf("command terminated with exit code %d", err2.ExitCode())
		}
		return errors.Fatal(err.Error())
	}
	return nil
}

func (fp *CommandReader) Close() error {
	if fp.waitHandled {
		return nil
	}

	return fp.wait()
}
