package sftp

import (
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/sftp"
)

// stabilityReset is how long after the last reconnect the reconnect counter
// starts again at 0.
const stabilityReset = 5 * time.Minute

// reconnectState limits how often the backend reconnects. The limit is there
// for a server that accepts every connection and drops it again shortly
// after: each reconnect succeeds, so without a limit the run would never end.
// mu also guards the c, cmd, result and posixRename fields of SFTP.
type reconnectState struct {
	mu            sync.Mutex
	reconnects    uint      // successful reconnects since the last reset
	lastReconnect time.Time // time of the last successful reconnect
	cause         error     // why the last connection was torn down
	closed        bool      // Close was called
}

// check returns a permanent error if the backend must not reconnect: reconnects
// are disabled (max == 0) or max reconnects happened without stabilityReset
// passing since the last one. Must be called with rs.mu held.
func (rs *reconnectState) check(max uint, now time.Time) error {
	if max == 0 {
		return rs.permanent("sftp: connection closed")
	}
	if !rs.lastReconnect.IsZero() && now.Sub(rs.lastReconnect) > stabilityReset {
		rs.reconnects = 0
	}
	if rs.reconnects >= max {
		return rs.permanent("sftp: reconnect limit reached")
	}
	return nil
}

// record counts a successful reconnect. Failed attempts are not counted:
// during an outage every pending operation fails one attempt per retry, which
// would use up the limit within seconds. The number of failed attempts is
// limited by the retry backend, which waits longer after each one and gives
// up after a fixed time. Must be called with rs.mu held.
func (rs *reconnectState) record(now time.Time) {
	rs.reconnects++
	rs.lastReconnect = now
}

func (rs *reconnectState) permanent(msg string) error {
	if rs.cause != nil {
		return backoff.Permanent(errors.Errorf("%s: %v", msg, rs.cause))
	}
	return backoff.Permanent(errors.New(msg))
}

// ensureConnection returns the client to use for one backend operation and
// whether it supports posix-rename. If the ssh process has exited or the
// connection was torn down by disconnect, it starts a new one, subject to the
// reconnect limit. The caller keeps using the returned client even if another
// goroutine replaces the connection in the meantime, and passes it to
// disconnect when the operation fails.
func (r *SFTP) ensureConnection() (*sftp.Client, bool, error) {
	r.rs.mu.Lock()
	defer r.rs.mu.Unlock()

	if r.rs.closed {
		return nil, false, backoff.Permanent(errors.New("sftp: backend is closed"))
	}

	if r.c != nil {
		select {
		case err := <-r.result:
			debug.Log("ssh process exited: %v", err)
			r.teardownLocked(err)
		default:
			return r.c, r.posixRename, nil
		}
	}

	now := time.Now()
	if err := r.rs.check(r.Config.Reconnect, now); err != nil {
		debug.Log("not reconnecting: %v", err)
		return nil, false, err
	}

	be, err := startClient(r.Config, r.errorLog)
	if err != nil {
		debug.Log("reconnect failed: %v", err)
		return nil, false, err
	}
	r.rs.record(now)

	r.c = be.c
	r.cmd = be.cmd
	r.result = be.result
	r.posixRename = be.posixRename

	debug.Log("reconnected (%d/%d)", r.rs.reconnects, r.Config.Reconnect)
	return r.c, r.posixRename, nil
}

// disconnect tears down the connection if err shows that it is dead, so that
// the next ensureConnection starts a new one. Any other error leaves the
// connection alone. c must be the client the failed operation used: if the
// backend has already moved on to a different connection, that one is healthy
// and is left alone as well.
func (r *SFTP) disconnect(c *sftp.Client, err error) {
	if !isConnectionLost(err) {
		return
	}

	r.rs.mu.Lock()
	defer r.rs.mu.Unlock()

	if r.c == nil || r.c != c {
		return
	}

	debug.Log("connection lost: %v", err)
	r.teardownLocked(err)
}

// teardownLocked closes the current connection and kills the ssh process if
// it does not exit within closeTimeout. cause is reported to the user if no
// reconnect is allowed afterwards. Must be called with r.rs.mu held and r.c
// non-nil.
func (r *SFTP) teardownLocked(cause error) {
	_ = r.c.Close()

	select {
	case err := <-r.result:
		// The exit status of ssh says more than the error of the operation
		// that noticed the dead connection.
		if err != nil {
			cause = err
		}
	case <-time.After(closeTimeout):
		if r.cmd != nil {
			_ = r.cmd.Process.Kill()
			//nolint:staticcheck // wait for the process to exit, the error is irrelevant
			_ = <-r.result
		}
	}

	r.rs.cause = cause
	r.c = nil
	r.cmd = nil
	r.result = nil
}

// isConnectionLost reports whether err is the error pkg/sftp returns for a
// dead connection. Once the client's receive loop has failed, every pending
// request and every later one gets ErrSSHFxConnectionLost
// (clientConn.broadcastErr and putChannel).
//
// Nothing else is matched on purpose. Errors the server sends (permission
// denied, no such file, failure) arrive as *sftp.StatusError and say nothing
// about the connection. A request sent before the receive loop has failed can
// return the write error instead (EPIPE). That means the ssh process has
// exited, which ensureConnection notices on the next operation. Matching such
// generic errors here would also match the same errors coming from the reader
// passed to Save or the consumer passed to Load.
func isConnectionLost(err error) bool {
	return errors.Is(err, sftp.ErrSSHFxConnectionLost)
}
