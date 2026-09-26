//go:build !windows

package terminal_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/restic/restic/internal/terminal"
	rtest "github.com/restic/restic/internal/test"
)

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestReadPasswordCancelNoRace exercises the cancellation path that used to
// race on the named err result when the leaked worker resumed after return.
func TestReadPasswordCancelNoRace(t *testing.T) {
	ptmx, pts, err := pty.Open()
	rtest.OK(t, err)
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = pts.Close() }()

	fd := int(pts.Fd())
	var out safeBuffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, callErr := terminal.ReadPassword(ctx, fd, &out, "password:")
		errCh <- callErr
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if bytes.Contains([]byte(out.String()), []byte("password:")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for password prompt; out=%q", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	select {
	case callErr := <-errCh:
		rtest.Assert(t, errors.Is(callErr, context.Canceled), "want context.Canceled, got %v", callErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for ReadPassword after cancel")
	}

	// Unblock the leaked worker so it can finish and send on the result channel.
	_, err = io.WriteString(ptmx, "secret\n")
	rtest.OK(t, err)

	// Give the worker a moment to resume; under -race this would trip if err were shared.
	time.Sleep(200 * time.Millisecond)
}
