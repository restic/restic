package sftp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/sftp"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/retry"
	"github.com/restic/restic/internal/errors"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

func TestIsConnectionLost(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},

		{name: "connection_lost", err: sftp.ErrSSHFxConnectionLost, want: true},
		{name: "wrapped_connection_lost", err: fmt.Errorf("Write foo: %w", sftp.ErrSSHFxConnectionLost), want: true},
		{name: "pkg_errors_wrapped_connection_lost", err: errors.Wrap(sftp.ErrSSHFxConnectionLost, "Lstat foo"), want: true},

		// The pkg/sftp client never returns ErrSSHFxNoConnection.
		{name: "no_connection", err: sftp.ErrSSHFxNoConnection, want: false},

		// A status the server sent says nothing about the connection, whatever
		// its code is.
		{name: "status_permission_denied", err: &sftp.StatusError{Code: uint32(sftp.ErrSSHFxPermissionDenied)}, want: false},
		{name: "status_no_such_file", err: &sftp.StatusError{Code: uint32(sftp.ErrSSHFxNoSuchFile)}, want: false},
		{name: "status_failure", err: &sftp.StatusError{Code: uint32(sftp.ErrSSHFxFailure)}, want: false},
		{name: "status_connection_lost", err: &sftp.StatusError{Code: uint32(sftp.ErrSSHFxConnectionLost)}, want: false},

		// These can also come from the reader passed to Save or the consumer
		// passed to Load, where they are unrelated to the connection.
		{name: "epipe", err: &os.PathError{Op: "write", Path: "|1", Err: syscall.EPIPE}, want: false},
		{name: "err_closed", err: &os.PathError{Op: "write", Path: "|1", Err: os.ErrClosed}, want: false},
		{name: "eof", err: io.EOF, want: false},
		{name: "unexpected_eof", err: io.ErrUnexpectedEOF, want: false},
		{name: "econnreset", err: syscall.ECONNRESET, want: false},

		// The message alone must not match.
		{name: "connection_lost_msg", err: fmt.Errorf("Write foo: connection lost"), want: false},
		{name: "ssh_exit_255_msg", err: fmt.Errorf("ssh command exited: exit status 255"), want: false},

		{name: "os_not_exist", err: os.ErrNotExist, want: false},
		{name: "os_permission", err: os.ErrPermission, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rtest.Equals(t, tt.want, isConnectionLost(tt.err))
		})
	}
}

func assertPermanent(t *testing.T, err error) {
	t.Helper()
	rtest.Assert(t, err != nil, "expected an error")
	rtest.Assert(t, errors.Is(err, &backoff.PermanentError{}), "expected permanent error, got %T: %v", err, err)
}

func TestReconnectStateDisabled(t *testing.T) {
	var rs reconnectState
	assertPermanent(t, rs.check(0, time.Now()))
}

func TestReconnectStateLimit(t *testing.T) {
	var rs reconnectState
	now := time.Now()

	for range 3 {
		rtest.OK(t, rs.check(3, now))
		rs.record(now)
	}
	assertPermanent(t, rs.check(3, now))
}

func TestReconnectStateFailedAttemptsAreFree(t *testing.T) {
	var rs reconnectState
	now := time.Now()

	// check without record is an attempt that did not produce a connection.
	for range 10 {
		rtest.OK(t, rs.check(1, now))
	}
	rs.record(now)
	assertPermanent(t, rs.check(1, now))
}

func TestReconnectStateReset(t *testing.T) {
	var rs reconnectState
	start := time.Now()

	rtest.OK(t, rs.check(1, start))
	rs.record(start)

	assertPermanent(t, rs.check(1, start.Add(stabilityReset)))

	rtest.OK(t, rs.check(1, start.Add(stabilityReset+time.Second)))
	rtest.Equals(t, uint(0), rs.reconnects)
}

func TestReconnectStateReportsCause(t *testing.T) {
	rs := reconnectState{cause: errors.New("ssh command exited: exit status 255")}
	err := rs.check(0, time.Now())
	assertPermanent(t, err)
	rtest.Assert(t, strings.Contains(err.Error(), "exit status 255"), "cause missing from %q", err)
}

func findServerBinary() string {
	for dir := range strings.SplitSeq(rtest.TestSFTPPath, ":") {
		testpath := filepath.Join(dir, "sftp-server")
		if _, err := os.Stat(testpath); err == nil {
			return testpath
		}
	}
	return ""
}

func reconnectTestBackend(t *testing.T, reconnect uint) *SFTP {
	server := findServerBinary()
	if server == "" {
		t.Skip("sftp server binary not found")
	}

	cfg := Config{
		Path:        filepath.Join(rtest.TempDir(t), "repo"),
		Command:     fmt.Sprintf("%q -e", server),
		Connections: 5,
		Reconnect:   reconnect,
	}

	be, err := Create(context.Background(), cfg, func(string, ...any) {})
	rtest.OK(t, err)
	t.Cleanup(func() { _ = be.Close() })
	return be
}

// killServer kills the process behind the backend's connection.
func killServer(t *testing.T, be *SFTP) {
	t.Helper()
	rtest.OK(t, be.cmd.Process.Kill())
}

// statAfterKill calls Stat until the backend has dealt with the killed
// process: Stat succeeds on a new connection or fails for good. Until the exit
// of the process is visible, calls still run on the dead connection and fail
// with a retryable error, which is either "connection lost" or the error of
// the failed write.
func statAfterKill(t *testing.T, be *SFTP, h backend.Handle) (backend.FileInfo, error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		fi, err := be.Stat(context.Background(), h)
		if err == nil || errors.Is(err, &backoff.PermanentError{}) || time.Now().After(deadline) {
			return fi, err
		}
		time.Sleep(time.Millisecond)
	}
}

func TestReconnectAfterProcessExit(t *testing.T) {
	be := reconnectTestBackend(t, 2)

	h := backend.Handle{Type: backend.PackFile, Name: "aa00000000000000000000000000000000000000000000000000000000000000"}
	data := []byte("test data")
	rtest.OK(t, be.Save(context.Background(), h, backend.NewByteReader(data, be.Hasher())))

	killServer(t, be)

	fi, err := statAfterKill(t, be, h)
	rtest.OK(t, err)
	rtest.Equals(t, int64(len(data)), fi.Size)
	rtest.Equals(t, uint(1), be.rs.reconnects)
}

func TestNoReconnectWhenDisabled(t *testing.T) {
	be := reconnectTestBackend(t, 0)

	killServer(t, be)

	_, err := statAfterKill(t, be, backend.Handle{Type: backend.ConfigFile})
	assertPermanent(t, err)
	rtest.Assert(t, strings.Contains(err.Error(), "ssh command exited"), "cause missing from %q", err)
}

func TestFailedReconnectIsRetried(t *testing.T) {
	be := reconnectTestBackend(t, 1)
	h := backend.Handle{Type: backend.ConfigFile}
	command := be.Config.Command

	// A command that exits without speaking sftp stands in for a server that
	// cannot be reached.
	be.Config.Command = "false"
	killServer(t, be)
	// Stat keeps working until the process is actually gone.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := be.Stat(context.Background(), h); err != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	for range 3 {
		_, err := be.Stat(context.Background(), h)
		rtest.Assert(t, err != nil, "expected an error")
		rtest.Assert(t, !errors.Is(err, &backoff.PermanentError{}), "failed attempt must stay retryable, got %v", err)
	}
	rtest.Equals(t, uint(0), be.rs.reconnects)

	be.Config.Command = command
	_, err := be.Stat(context.Background(), h)
	rtest.Assert(t, be.IsNotExist(err), "expected not-exist from a working connection, got %v", err)
	rtest.Equals(t, uint(1), be.rs.reconnects)
}

func TestNoReconnectAfterClose(t *testing.T) {
	be := reconnectTestBackend(t, 2)

	rtest.OK(t, be.Close())

	_, err := be.Stat(context.Background(), backend.Handle{Type: backend.ConfigFile})
	assertPermanent(t, err)
	rtest.Equals(t, uint(0), be.rs.reconnects)
}

// TestReconnectDuringParallelSaves kills the server while several goroutines
// save through the retry backend. Every save has to succeed, and the workers
// that lose the same connection have to share one reconnect.
func TestReconnectDuringParallelSaves(t *testing.T) {
	be := reconnectTestBackend(t, 5)
	rbe := retry.New(be, time.Minute, nil, nil)

	const workers, savesPerWorker = 5, 40
	data := []byte("test data")
	name := func(w, i int) string {
		return fmt.Sprintf("%02x%062x", w, i)
	}

	firstSave := make(chan struct{})
	var once sync.Once
	var g errgroup.Group
	for w := range workers {
		g.Go(func() error {
			for i := range savesPerWorker {
				h := backend.Handle{Type: backend.PackFile, Name: name(w, i)}
				if err := rbe.Save(context.Background(), h, backend.NewByteReader(data, be.Hasher())); err != nil {
					return err
				}
				once.Do(func() { close(firstSave) })
			}
			return nil
		})
	}

	<-firstSave
	be.rs.mu.Lock()
	cmd := be.cmd
	be.rs.mu.Unlock()
	rtest.OK(t, cmd.Process.Kill())

	rtest.OK(t, g.Wait())

	be.rs.mu.Lock()
	reconnects := be.rs.reconnects
	be.rs.mu.Unlock()
	rtest.Equals(t, uint(1), reconnects)

	for w := range workers {
		for i := range savesPerWorker {
			fi, err := be.Stat(context.Background(), backend.Handle{Type: backend.PackFile, Name: name(w, i)})
			rtest.OK(t, err)
			rtest.Equals(t, int64(len(data)), fi.Size)
		}
	}
}
