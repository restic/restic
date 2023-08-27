package fs

import (
	"io"
	"os/exec"
)

// ReadCloserCommand wraps an exec.Cmd and its standard output to provide an
// io.ReadCloser that waits for the command to terminate on Close(), reporting
// any error in the command.Wait() function back to the Close() caller.
type ReadCloserCommand struct {
	Cmd    *exec.Cmd
	Stdout io.ReadCloser
}

func (fp *ReadCloserCommand) Read(p []byte) (n int, err error) {
	return fp.Stdout.Read(p)
}

func (fp *ReadCloserCommand) Close() error {
	// No need to close fp.Stdout as Wait() closes all pipes.
	return fp.Cmd.Wait()
}
