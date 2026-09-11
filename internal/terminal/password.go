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
func ReadPassword(ctx context.Context, inFd int, out io.Writer, prompt string) (string, error) {
	state, err := term.GetState(inFd)
	if err != nil {
		_, _ = fmt.Fprintf(out, "unable to get terminal state: %v\n", err)
		return "", err
	}

	type result struct {
		password string
		err      error
	}
	// Buffered so a leaked worker after cancellation can still send without blocking forever.
	done := make(chan result, 1)

	go func() {
		_, readErr := fmt.Fprint(out, prompt)
		if readErr != nil {
			done <- result{err: readErr}
			return
		}
		buf, readErr := term.ReadPassword(inFd)
		if readErr != nil {
			done <- result{err: readErr}
			return
		}
		_, readErr = fmt.Fprintln(out)
		if readErr != nil {
			done <- result{err: readErr}
			return
		}
		done <- result{password: string(buf)}
	}()

	select {
	case <-ctx.Done():
		restoreErr := term.Restore(inFd, state)
		if restoreErr != nil {
			_, _ = fmt.Fprintf(out, "unable to restore terminal state: %v\n", restoreErr)
		}
		return "", ctx.Err()
	case res := <-done:
		if res.err != nil {
			return "", fmt.Errorf("ReadPassword: %w", res.err)
		}
		return res.password, nil
	}
}
