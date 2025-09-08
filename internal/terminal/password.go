package terminal

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/term"
)

// ReadPassword reads the password from the given reader which must be a
// tty. Prompt is printed on the writer out before attempting to read the
// password. If the context is canceled, the function leaks the password reading
// goroutine.
func ReadPassword(ctx context.Context, in *os.File, out *os.File, prompt string) (password string, err error) {
	fd := int(out.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to get terminal state: %v\n", err)
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
		buf, err = term.ReadPassword(int(in.Fd()))
		if err != nil {
			return
		}
		_, err = fmt.Fprintln(out)
	}()

	select {
	case <-ctx.Done():
		err := term.Restore(fd, state)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "unable to restore terminal state: %v\n", err)
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
