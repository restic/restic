package sftp

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"github.com/restic/restic/internal/errors"
	rtest "github.com/restic/restic/internal/test"
)

func TestIsTransientDisconnect(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// Nil error
		{name: "nil", err: nil, want: false},

		// SFTP status errors — transient
		{
			name: "sftp_connection_lost",
			err:  &sftp.StatusError{Code: uint32(sftp.ErrSSHFxConnectionLost)},
			want: true,
		},
		{
			name: "sftp_no_connection",
			err:  &sftp.StatusError{Code: uint32(sftp.ErrSSHFxNoConnection)},
			want: true,
		},

		// SFTP status errors — permanent
		{
			name: "sftp_permission_denied",
			err:  &sftp.StatusError{Code: uint32(sftp.ErrSSHFxPermissionDenied)},
			want: false,
		},
		{
			name: "sftp_no_such_file",
			err:  &sftp.StatusError{Code: uint32(sftp.ErrSSHFxNoSuchFile)},
			want: false,
		},
		{
			name: "sftp_failure",
			err:  &sftp.StatusError{Code: uint32(sftp.ErrSSHFxFailure)},
			want: false,
		},
		{
			name: "sftp_op_unsupported",
			err:  &sftp.StatusError{Code: uint32(sftp.ErrSSHFxOpUnsupported)},
			want: false,
		},

		// io errors
		{name: "unexpected_eof", err: io.ErrUnexpectedEOF, want: true},
		{name: "regular_eof", err: io.EOF, want: false},

		// Syscall errors
		{name: "epipe", err: syscall.EPIPE, want: true},
		{name: "econnreset", err: syscall.ECONNRESET, want: true},
		{name: "econnaborted", err: syscall.ECONNABORTED, want: true},
		{name: "enoent", err: syscall.ENOENT, want: false},
		{name: "eacces", err: syscall.EACCES, want: false},

		// SSH exit status 255 — transient
		{
			name: "ssh_exit_255",
			err:  fmt.Errorf("ssh command exited: exit status 255"),
			want: true,
		},
		{
			name: "wrapped_ssh_exit_255",
			err:  errors.Wrap(fmt.Errorf("ssh command exited: exit status 255"), "operation failed"),
			want: true,
		},

		// SSH exit with other status codes — NOT transient
		{
			name: "ssh_exit_0",
			err:  fmt.Errorf("ssh command exited: exit status 0"),
			want: false,
		},
		{
			name: "ssh_exit_1",
			err:  fmt.Errorf("ssh command exited: exit status 1"),
			want: false,
		},
		{
			name: "ssh_exit_127",
			err:  fmt.Errorf("ssh command exited: exit status 127"),
			want: false,
		},

		// String-matched network errors
		{name: "broken_pipe_msg", err: fmt.Errorf("write: broken pipe"), want: true},
		{name: "conn_reset_msg", err: fmt.Errorf("read: connection reset by peer"), want: true},
		{name: "connection_lost_msg", err: fmt.Errorf("Write foo: connection lost"), want: true},
		{name: "file_already_closed_msg", err: fmt.Errorf("failed to send packet: write |1: file already closed"), want: true},

		// Raw fxerr sentinel values (broadcastErr path in pkg/sftp)
		{name: "fxerr_connection_lost", err: sftp.ErrSSHFxConnectionLost, want: true},
		{name: "fxerr_no_connection", err: sftp.ErrSSHFxNoConnection, want: true},
		{name: "wrapped_fxerr_connection_lost", err: fmt.Errorf("Write foo: %w", sftp.ErrSSHFxConnectionLost), want: true},
		{name: "wrapped_fxerr_no_connection", err: fmt.Errorf("OpenFile foo: %w", sftp.ErrSSHFxNoConnection), want: true},

		// Wrapped transient errors
		{
			name: "wrapped_epipe",
			err:  fmt.Errorf("Save: %w", syscall.EPIPE),
			want: true,
		},
		{
			name: "wrapped_unexpected_eof",
			err:  fmt.Errorf("Read: %w", io.ErrUnexpectedEOF),
			want: true,
		},

		// Permanent errors that must not be classified as transient
		{name: "os_not_exist", err: os.ErrNotExist, want: false},
		{name: "os_permission", err: os.ErrPermission, want: false},
		{name: "generic_error", err: fmt.Errorf("some other error"), want: false},
		{name: "no_space", err: fmt.Errorf("sftp: no space left on device"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientDisconnect(tt.err)
			rtest.Equals(t, tt.want, got)
		})
	}
}

func TestReconnectBudget(t *testing.T) {
	t.Run("basic_consumption", func(t *testing.T) {
		b := newReconnectBudget()
		ok, n := b.tryConsume(3)
		rtest.Assert(t, ok, "attempt 1 should succeed")
		rtest.Equals(t, uint(1), n)

		ok, n = b.tryConsume(3)
		rtest.Assert(t, ok, "attempt 2 should succeed")
		rtest.Equals(t, uint(2), n)

		ok, n = b.tryConsume(3)
		rtest.Assert(t, ok, "attempt 3 should succeed")
		rtest.Equals(t, uint(3), n)

		ok, _ = b.tryConsume(3)
		rtest.Assert(t, !ok, "attempt 4 should fail (budget exhausted)")
	})

	t.Run("zero_budget", func(t *testing.T) {
		b := newReconnectBudget()
		ok, _ := b.tryConsume(0)
		rtest.Assert(t, !ok, "zero budget should always fail")
	})

	t.Run("stability_reset", func(t *testing.T) {
		b := newReconnectBudget()
		b.stabilityReset = 10 * time.Millisecond // short for testing

		ok, _ := b.tryConsume(2)
		rtest.Assert(t, ok, "attempt 1 should succeed")
		ok, _ = b.tryConsume(2)
		rtest.Assert(t, ok, "attempt 2 should succeed")
		ok, _ = b.tryConsume(2)
		rtest.Assert(t, !ok, "attempt 3 should fail")

		// Wait for stability window.
		time.Sleep(20 * time.Millisecond)

		// Budget should be reset.
		ok, _ = b.tryConsume(2)
		rtest.Assert(t, ok, "after stability reset, attempt should succeed")
		ok, _ = b.tryConsume(2)
		rtest.Assert(t, ok, "after stability reset, second attempt should succeed")
	})

	t.Run("elapsed_time_limit", func(t *testing.T) {
		b := newReconnectBudget()
		b.maxElapsedTime = 10 * time.Millisecond // short for testing

		ok, _ := b.tryConsume(100)
		rtest.Assert(t, ok, "first attempt should succeed")

		// Wait past max elapsed time.
		time.Sleep(20 * time.Millisecond)

		ok, _ = b.tryConsume(100)
		rtest.Assert(t, !ok, "should fail after max elapsed time")
	})
}

func TestReconnectBackoff(t *testing.T) {
	// Attempt 1: backoff in [0, 250ms]
	for i := 0; i < 100; i++ {
		d := reconnectBackoff(1)
		rtest.Assert(t, d >= 0 && d <= 250*time.Millisecond,
			"attempt 1 backoff %v out of range", d)
	}

	// Attempt 2: backoff in [0, 500ms]
	for i := 0; i < 100; i++ {
		d := reconnectBackoff(2)
		rtest.Assert(t, d >= 0 && d <= 500*time.Millisecond,
			"attempt 2 backoff %v out of range", d)
	}

	// Attempt 3: backoff in [0, 1s]
	for i := 0; i < 100; i++ {
		d := reconnectBackoff(3)
		rtest.Assert(t, d >= 0 && d <= time.Second,
			"attempt 3 backoff %v out of range", d)
	}

	// Large attempt: backoff capped at [0, 15s]
	for i := 0; i < 100; i++ {
		d := reconnectBackoff(100)
		rtest.Assert(t, d >= 0 && d <= 15*time.Second,
			"attempt 100 backoff %v out of range", d)
	}
}

func TestReconnectBudgetConcurrent(t *testing.T) {
	b := newReconnectBudget()
	maxAttempts := uint(10)

	var consumed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _ := b.tryConsume(maxAttempts)
			if ok {
				consumed.Add(1)
			}
		}()
	}
	wg.Wait()

	rtest.Equals(t, int32(maxAttempts), consumed.Load())
}

func TestEnsureConnectedAfterClose(t *testing.T) {
	r := &SFTP{
		budget: newReconnectBudget(),
		Config: Config{Reconnect: 5},
	}
	r.closed.Store(true)

	_, err := r.ensureConnected(0)
	rtest.Assert(t, err != nil, "ensureConnected should fail after Close()")
	rtest.Assert(t, strings.Contains(err.Error(), "backend is closed"),
		"error should indicate backend is closed, got: %v", err)
}

func TestWithRetryAfterClose(t *testing.T) {
	r := &SFTP{
		budget: newReconnectBudget(),
		Config: Config{Reconnect: 5},
	}
	// conn_ is nil and closed is set — should get permanent error, not reconnect.
	r.closed.Store(true)

	called := false
	err := r.withRetry(func(cs *connState) error {
		called = true
		return nil
	})
	rtest.Assert(t, !called, "fn should not be called after Close()")
	rtest.Assert(t, err != nil, "withRetry should fail after Close()")
	rtest.Assert(t, strings.Contains(err.Error(), "backend is closed"),
		"error should indicate backend is closed, got: %v", err)
}
