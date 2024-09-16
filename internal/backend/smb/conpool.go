package smb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/cloudsoda/go-smb2"
	"github.com/restic/restic/internal/debug"
)

// Parts of this code have been adapted from Rclone (https://github.com/rclone)
// Copyright (C) 2012 by Nick Craig-Wood http://www.craig-wood.com/nick/

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// conn encapsulates a SMB client and corresponding SMB session and share.
type conn struct {
	netConn    net.Conn
	smbSession *smb2.Session
	smbShare   *smb2.Share
	shareName  string
}

func (c *conn) close() error {
	var errs []error
	if c.smbShare != nil {
		errs = append(errs, c.smbShare.Umount())
	}
	if c.smbSession != nil {
		errs = append(errs, c.smbSession.Logoff())
	}
	return errors.Join(errs...)
}

// isClosed checks if the connection is closed.
func (c *conn) isClosed() bool {
	if c.smbShare != nil {
		// stat the current directory
		_, err := c.smbShare.Stat(".")
		return err != nil
	}
	// list the shares
	_, err := c.smbSession.ListSharenames()
	return err != nil
}

// addSession increments the active session count when an SMB session needs to be used.
// If this is called, we must call removeSession when we are done using the session.
func (b *SMB) addSession() {
	b.sessions.Add(1)
}

// removeSession decrements the active session count when it is no longer in use.
func (b *SMB) removeSession() {
	b.sessions.Add(-1)
}

// getSessionCount returns the number of active sessions.
func (b *SMB) getSessionCount() int32 {
	return b.sessions.Load()
}

// dial starts a client connection to the given SMB server. It is a
// convenience function that connects to the given network address,
// initiates the SMB handshake, and then returns a session for SMB communication.
func (b *SMB) dial(ctx context.Context, network, addr string) (*conn, error) {
	dialer := net.Dialer{}
	netConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("SMB dial failed: %w", err)
	}

	clientID := b.getClientID()

	d := &smb2.Dialer{
		Negotiator: smb2.Negotiator{
			RequireMessageSigning: b.RequireMessageSigning,
			SpecifiedDialect:      b.Dialect,
			ClientGuid:            clientID,
		},
		Initiator: &smb2.NTLMInitiator{
			User:      b.User,
			Password:  b.Password.Unwrap(),
			Domain:    b.Domain,
			TargetSPN: b.SPN,
		},
	}

	session, err := d.DialConn(ctx, netConn, addr)
	if err != nil {
		return nil, fmt.Errorf("SMB session initialization failed: %w", err)
	}

	return &conn{
		netConn:    netConn,
		smbSession: session,
	}, nil
}

// getClientID returns the client GUID.
func (b *SMB) getClientID() [16]byte {
	var clientID [16]byte
	if b.ClientGUID != "" {
		copy(clientID[:], []byte(b.ClientGUID))
	}
	return clientID
}

// newConnection creates a new SMB connection.
func (b *SMB) newConnection(share string) (*conn, error) {
	// As we are pooling these connections we need to decouple
	// them from the current context
	ctx := context.Background()

	c, err := b.dial(ctx, "tcp", net.JoinHostPort(b.Host, strconv.Itoa(b.Port)))
	if err != nil {
		return nil, fmt.Errorf("SMB connection failed: %w", err)
	}

	if share != "" {
		// mount the specified share as well if user requested
		c.smbShare, err = c.smbSession.Mount(share)
		if err != nil {
			_ = c.smbSession.Logoff()
			return nil, fmt.Errorf("SMB share mount failed: %w", err)
		}
		c.smbShare = c.smbShare.WithContext(ctx)
	}

	return c, nil
}

// mountShare ensures the existing share is unmounted and the specified share is mounted.
func (c *conn) mountShare(share string) error {
	if c.shareName == share {
		return nil
	}
	if c.smbShare != nil {
		if err := c.smbShare.Umount(); err != nil {
			// Check if we should not nil out the share for some errors
			c.smbShare = nil
			return err
		}
		c.smbShare = nil
	}
	if share != "" {
		var err error
		c.smbShare, err = c.smbSession.Mount(share)
		if err != nil {
			return err
		}
	}
	c.shareName = share
	return nil
}

// getConnection retrieves a connection from the pool or creates a new one.
func (b *SMB) getConnection(share string) (c *conn, err error) {
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
	return b.newConnection(share)
}

// putConnection returns a connection to the pool for reuse.
func (b *SMB) putConnection(c *conn) {
	if c == nil {
		return
	}

	if c.isClosed() {
		debug.Log("Connection closed, not returning to pool")
		_ = c.close()
		return
	}

	b.poolMu.Lock()
	defer b.poolMu.Unlock()

	b.pool = append(b.pool, c)
	b.drain.Reset(b.IdleTimeout)
}

// drainPool closes all unused connections in the pool.
func (b *SMB) drainPool() error {
	b.poolMu.Lock()
	defer b.poolMu.Unlock()

	if sessions := b.getSessionCount(); sessions != 0 {
		debug.Log("Not closing %d unused connections as %d sessions active", len(b.pool), sessions)
		b.drain.Reset(b.IdleTimeout) // reset the timer to keep the pool open
		return nil
	}

	if b.IdleTimeout > 0 {
		b.drain.Stop()
	}
	if len(b.pool) != 0 {
		debug.Log("Attempting to close %d unused connections", len(b.pool))
	}
	var errs []error
	for _, c := range b.pool {
		if !c.isClosed() {
			if err := c.close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	b.pool = nil

	return errors.Join(errs...)
}
