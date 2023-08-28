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

	bytesRead bool
}

// Read populate the array with data from the process stdout.
func (fp *ReadCloserCommand) Read(p []byte) (int, error) {
	// We may encounter two different error conditions here:
	// - EOF with no bytes read: the program terminated prematurely, so we send
	//   a fatal error to cancel the snapshot;
	// - an error that is not EOF: something bad happened, we need to abort the
	//   snapshot.
	b, err := fp.Stdout.Read(p)
	if b == 0 && errors.Is(err, io.EOF) && !fp.bytesRead {
		// The command terminated with no output at all. Raise a fatal error.
		return 0, errors.Fatalf("command terminated with no output")
	} else if err != nil && !errors.Is(err, io.EOF) {
		// The command terminated with an error that is not EOF. Raise a fatal
		// error.
		return 0, errors.Fatal(err.Error())
	} else if b > 0 {
		fp.bytesRead = true
	}
	return b, err
}

func (fp *ReadCloserCommand) Close() error {
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
