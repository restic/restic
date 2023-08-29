package fs

import (
	"github.com/restic/restic/internal/errors"
	"io"
	"os/exec"
)

// ReadCloserCommand wraps an exec.Cmd and its standard output to provide an
// io.ReadCloser that waits for the command to terminate on Close(), reporting
// any error in the command.Wait() function back to the Close() caller.
type ReadCloserCommand struct {
	Cmd    *exec.Cmd
	Stdout io.ReadCloser

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

// Read populate the array with data from the process stdout.
func (fp *ReadCloserCommand) Read(p []byte) (int, error) {
	if fp.alreadyClosedReadErr != nil {
		return 0, fp.alreadyClosedReadErr
	}
	b, err := fp.Stdout.Read(p)

	// If the error is io.EOF, the program terminated. We need to check the
	// exit code here because, if the program terminated with no output, the
	// error in `Close()` is ignored.
	if errors.Is(err, io.EOF) {
		// Check if the command terminated successfully. If not, return the
		// error.
		fp.waitHandled = true
		errw := fp.Cmd.Wait()
		if errw != nil {
			// If we have information about the exit code, let's use it in the
			// error message. Otherwise, send the error message along.
			// In any case, use a fatal error to abort the snapshot.
			var err2 *exec.ExitError
			if errors.As(errw, &err2) {
				err = errors.Fatalf("command terminated with exit code %d", err2.ExitCode())
			} else {
				err = errors.Fatal(errw.Error())
			}
		}
	}
	fp.alreadyClosedReadErr = err
	return b, err
}

func (fp *ReadCloserCommand) Close() error {
	if fp.waitHandled {
		return nil
	}

	// No need to close fp.Stdout as Wait() closes all pipes.
	err := fp.Cmd.Wait()
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
