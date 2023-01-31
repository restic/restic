package smb

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"

	"github.com/hirochachacha/go-smb2"
	"github.com/restic/restic/internal/debug"
)

// conn encapsulates a SMB client and corresponding SMB client
type conn struct {
	conn       *net.Conn
	smbSession *smb2.Session
	smbShare   *smb2.Share
	shareName  string
}

// Closes the connection
func (c *conn) close() (err error) {
	if c.smbShare != nil {
		err = c.smbShare.Umount()
	}
	sessionLogoffErr := c.smbSession.Logoff()
	if err != nil {
		return err
	}
	return sessionLogoffErr
}

// True if it's closed
func (c *conn) closed() bool {
	var nopErr error
	if c.smbShare != nil {
		// stat the current directory
		_, nopErr = c.smbShare.Stat(".")
	} else {
		// list the shares
		_, nopErr = c.smbSession.ListSharenames()
	}
	return nopErr == nil
}

// Show that we are using a SMB session
//
// Call removeSession() when done
func (b *Backend) addSession() {
	atomic.AddInt32(&b.sessions, 1)
}

// Show the SMB session is no longer in use
func (b *Backend) removeSession() {
	atomic.AddInt32(&b.sessions, -1)
}

// getSessions shows whether there are any sessions in use
func (b *Backend) getSessions() int32 {
	return atomic.LoadInt32(&b.sessions)
}

// dial starts a client connection to the given SMB server. It is a
// convenience function that connects to the given network address,
// initiates the SMB handshake, and then sets up a Client.
func (b *Backend) dial(ctx context.Context, network, addr string) (*conn, error) {
	dialer := net.Dialer{}
	tconn, err := dialer.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	var clientId [16]byte
	if b.ClientGuid != "" {
		copy(clientId[:], []byte(b.ClientGuid))
	}

	d := &smb2.Dialer{
		Negotiator: smb2.Negotiator{
			RequireMessageSigning: b.RequireMessageSigning,
			SpecifiedDialect:      b.Dialect,
			ClientGuid:            clientId,
		},
		Initiator: &smb2.NTLMInitiator{
			User:     b.User,
			Password: b.Password.Unwrap(),
			Domain:   b.Domain,
		},
	}

	session, err := d.DialContext(ctx, tconn)
	if err != nil {
		return nil, err
	}

	return &conn{
		smbSession: session,
		conn:       &tconn,
	}, nil
}

// Open a new connection to the SMB server.
func (b *Backend) newConnection(share string) (c *conn, err error) {
	// As we are pooling these connections we need to decouple
	// them from the current context
	ctx := context.Background()

	c, err = b.dial(ctx, "tcp", b.Host+":"+strconv.Itoa(b.Port))
	if err != nil {
		return nil, fmt.Errorf("couldn't connect SMB: %w", err)
	}

	if share != "" {
		// mount the specified share as well if user requested
		c.smbShare, err = c.smbSession.Mount(share)
		if err != nil {
			_ = c.smbSession.Logoff()
			return nil, fmt.Errorf("couldn't initialize SMB: %w", err)
		}
		c.smbShare = c.smbShare.WithContext(ctx)
	}

	return c, nil
}

// Ensure the specified share is mounted or the session is unmounted
func (c *conn) mountShare(share string) (err error) {
	if c.shareName == share {
		return nil
	}
	if c.smbShare != nil {
		err = c.smbShare.Umount()
		c.smbShare = nil
	}
	if err != nil {
		return
	}
	if share != "" {
		c.smbShare, err = c.smbSession.Mount(share)
		if err != nil {
			return
		}
	}
	c.shareName = share
	return nil
}

// Get a SMB connection from the pool, or open a new one
func (b *Backend) getConnection(ctx context.Context, share string) (c *conn, err error) {
	b.poolMu.Lock()
	for len(b.pool) > 0 {
		c = b.pool[0]
		b.pool = b.pool[1:]
		err = c.mountShare(share)
		if err == nil {
			break
		}
		debug.Log("Discarding unusable SMB connection: %v", err)
		c = nil
	}
	b.poolMu.Unlock()
	if c != nil {
		return c, nil
	}
	c, err = b.newConnection(share)
	return c, err
}

// Return a SMB connection to the pool
//
// It nils the pointed to connection out so it can't be reused
func (b *Backend) putConnection(pc **conn) {
	c := *pc
	*pc = nil

	var nopErr error
	if c.smbShare != nil {
		// stat the current directory
		_, nopErr = c.smbShare.Stat(".")
	} else {
		// list the shares
		_, nopErr = c.smbSession.ListSharenames()
	}
	if nopErr != nil {
		debug.Log("Connection failed, closing: %v", nopErr)
		_ = c.close()
		return
	}

	b.poolMu.Lock()
	b.pool = append(b.pool, c)
	b.drain.Reset(b.Config.IdleTimeout) // nudge on the pool emptying timer
	b.poolMu.Unlock()
}

// Drain the pool of any connections
func (b *Backend) drainPool() (err error) {
	b.poolMu.Lock()
	defer b.poolMu.Unlock()
	if sessions := b.getSessions(); sessions != 0 {
		debug.Log("Not closing %d unused connections as %d sessions active", len(b.pool), sessions)
		b.drain.Reset(b.Config.IdleTimeout) // nudge on the pool emptying timer
		return nil
	}
	if b.Config.IdleTimeout > 0 {
		b.drain.Stop()
	}
	if len(b.pool) != 0 {
		debug.Log("Closing %d unused connections", len(b.pool))
	}
	for i, c := range b.pool {
		if !c.closed() {
			cErr := c.close()
			if cErr != nil {
				err = cErr
			}
		}
		b.pool[i] = nil
	}
	b.pool = nil
	return err
}
