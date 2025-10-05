package terminal

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/term"
)

// ReadPassword reads the password from the given reader which must be a
// tty. Prompt is printed on the writer out before attempting to read the
// password. If the context is canceled, the function leaks the password reading
// goroutine.
func ReadPassword(ctx context.Context, inFd int, out io.Writer, prompt string) (password string, err error) {
	state, err := term.GetState(inFd)
	if err != nil {
		_, _ = fmt.Fprintf(out, "unable to get terminal state: %v\n", err)
		return "", err
	}

	done := make(chan struct{})
	var buf []byte

	go func() {
		defer close(done)
		_, err = fmt.Fprint(out, prompt)
		if err != nil {
			return
		}
		buf, err = term.ReadPassword(inFd)
		if err != nil {
			return
		}
		_, err = fmt.Fprintln(out)
	}()

	select {
	case <-ctx.Done():
		err := term.Restore(inFd, state)
		if err != nil {
			_, _ = fmt.Fprintf(out, "unable to restore terminal state: %v\n", err)
		}
		return "", ctx.Err()
	case <-done:
		// clean shutdown, nothing to do
	}

	if err != nil {
		return "", fmt.Errorf("ReadPassword: %w", err)
	}

	return string(buf), nil
}
