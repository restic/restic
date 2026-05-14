package sftp

import (
	"io"
	"math/rand"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/sftp"
)

// connState holds the per-connection state for an SFTP backend.
// Each reconnect creates a new connState and tears down the old one.
type connState struct {
	client      *sftp.Client
	cmd         *os.Process
	result      <-chan error
	posixRename bool
	done        chan struct{} // closed on teardown to stop goroutines
	closeOnce   sync.Once
}

// close tears down the connection. Safe to call multiple times.
func (cs *connState) close() {
	cs.closeOnce.Do(func() {
		_ = cs.client.Close()

		// Wait for the SSH process to exit. If it doesn't exit within
		// closeTimeout, kill it and wait for the result.
		select {
		case <-cs.result:
		case <-time.After(closeTimeout):
			_ = cs.cmd.Kill()
			<-cs.result
		}

		// Now that cmd.Wait() has completed, signal auxiliary goroutines
		// (stderr reader) to stop.
		close(cs.done)
	})
}

// reconnectBudget tracks global reconnect attempts per backend instance.
type reconnectBudget struct {
	mu             sync.Mutex
	attempts       uint
	lastReconnect  time.Time
	firstReconnect time.Time
	maxElapsedTime time.Duration
	stabilityReset time.Duration
}

const (
	defaultMaxElapsedTime = 15 * time.Minute
	defaultStabilityReset = 5 * time.Minute

	reconnectInitialBackoff = 250 * time.Millisecond
	reconnectMaxBackoff     = 15 * time.Second
	reconnectBackoffFactor  = 2.0
)

var errBackendClosed = backoff.Permanent(errors.New("sftp: backend is closed"))

func newReconnectBudget() reconnectBudget {
	return reconnectBudget{
		maxElapsedTime: defaultMaxElapsedTime,
		stabilityReset: defaultStabilityReset,
	}
}

// tryConsume checks if a reconnect attempt is allowed.
func (b *reconnectBudget) tryConsume(maxAttempts uint) (bool, uint) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Reset counter if connection has been stable for stabilityReset.
	if !b.lastReconnect.IsZero() && now.Sub(b.lastReconnect) > b.stabilityReset {
		b.attempts = 0
		b.firstReconnect = time.Time{}
	}

	if b.attempts >= maxAttempts {
		return false, 0
	}
	if !b.firstReconnect.IsZero() && now.Sub(b.firstReconnect) > b.maxElapsedTime {
		return false, 0
	}

	if b.firstReconnect.IsZero() {
		b.firstReconnect = now
	}
	b.attempts++
	b.lastReconnect = now
	return true, b.attempts
}

// reconnectBackoff returns the backoff duration for the given attempt number
// (1-based), using exponential backoff with full jitter.
func reconnectBackoff(attempt uint) time.Duration {
	dur := reconnectInitialBackoff
	for i := uint(1); i < attempt; i++ {
		dur = time.Duration(float64(dur) * reconnectBackoffFactor)
		if dur > reconnectMaxBackoff {
			dur = reconnectMaxBackoff
			break
		}
	}
	return time.Duration(rand.Int63n(int64(dur) + 1))
}

// conn returns the current connection state under lock.
func (r *SFTP) conn() *connState {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	return r.cs
}

// ensureConnected reconnects after a transient disconnect. It uses
// singleflight to coalesce concurrent reconnect requests.
// preGen is the generation counter observed before the failing operation.
func (r *SFTP) ensureConnected(preGen uint64) (*connState, error) {
	v, err, _ := r.sfGroup.Do("reconnect", func() (interface{}, error) {
		if r.closed.Load() {
			return nil, errBackendClosed
		}

		// Another goroutine already reconnected.
		if r.connGen.Load() != preGen {
			return r.conn(), nil
		}

		ok, attempt := r.budget.tryConsume(r.Config.Reconnect)
		if !ok {
			debug.Log("reconnect budget exhausted (max %d attempts)", r.Config.Reconnect)
			return nil, backoff.Permanent(errors.New("sftp: reconnect budget exhausted"))
		}

		delay := reconnectBackoff(attempt)
		debug.Log("reconnecting (attempt %d/%d) after %v backoff", attempt, r.Config.Reconnect, delay)
		time.Sleep(delay)

		if r.closed.Load() {
			return nil, errBackendClosed
		}

		// Tear down old connection.
		r.connMu.Lock()
		old := r.cs
		r.cs = nil
		r.connMu.Unlock()

		if old != nil {
			old.close()
		}

		cs, err := startClient(r.Config, r.errorLog)
		if err != nil {
			debug.Log("reconnect failed: %v", err)
			return nil, err
		}

		if r.closed.Load() {
			cs.close()
			return nil, errBackendClosed
		}

		r.connMu.Lock()
		r.cs = cs
		r.connGen.Add(1)
		r.connMu.Unlock()

		debug.Log("reconnect succeeded (attempt %d/%d, gen %d)",
			attempt, r.Config.Reconnect, r.connGen.Load())
		return cs, nil
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errBackendClosed
	}
	return v.(*connState), nil
}

// getConn returns a usable connection and its generation counter.
// If the current connection is nil and reconnect is enabled, it joins
// an in-progress reconnect via singleflight.
func (r *SFTP) getConn() (*connState, uint64, error) {
	r.connMu.Lock()
	cs, gen := r.cs, r.connGen.Load()
	r.connMu.Unlock()

	if cs != nil {
		return cs, gen, nil
	}

	if r.Config.Reconnect > 0 && !r.closed.Load() {
		cs, err := r.ensureConnected(gen)
		if err != nil {
			return nil, 0, err
		}
		return cs, r.connGen.Load(), nil
	}

	return nil, 0, errBackendClosed
}

// withRetry runs fn with the current connection. On transient disconnect
// with reconnect enabled, it reconnects and retries fn once.
func (r *SFTP) withRetry(fn func(cs *connState) error) error {
	cs, gen, err := r.getConn()
	if err != nil {
		return err
	}

	err = fn(cs)
	if err == nil || r.Config.Reconnect == 0 || !isTransientDisconnect(err) {
		return err
	}

	debug.Log("transient disconnect, attempting reconnect (gen %d): %v", gen, err)

	newCS, reconnErr := r.ensureConnected(gen)
	if reconnErr != nil {
		return reconnErr
	}

	return fn(newCS)
}

// isTransientDisconnect returns true if the error indicates a transient
// SSH/SFTP connection loss that may be recovered by reconnecting.
func isTransientDisconnect(err error) bool {
	if err == nil {
		return false
	}

	// SFTP status errors (server-side SSH_FXP_STATUS responses).
	var statusErr *sftp.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.FxCode() {
		case sftp.ErrSSHFxConnectionLost, sftp.ErrSSHFxNoConnection:
			return true
		}
	}

	// Raw fxerr sentinels broadcast by pkg/sftp when the SSH pipe breaks.
	// These are fxerr (uint32), not *StatusError.
	if errors.Is(err, sftp.ErrSSHFxConnectionLost) ||
		errors.Is(err, sftp.ErrSSHFxNoConnection) {
		return true
	}

	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	if errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) {
		return true
	}

	// String fallbacks for SSH transport death indicators.
	msg := err.Error()
	if strings.Contains(msg, "ssh command exited: exit status 255") {
		return true
	}
	if strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "connection lost") ||
		strings.Contains(msg, "file already closed") {
		return true
	}

	return false
}