package rclone

import (
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
)

// StdioConn implements a net.Conn via stdin/stdout.
type StdioConn struct {
	receive   *os.File
	send      *os.File
	cmd       *exec.Cmd
	closeRecv sync.Once
	closeSend sync.Once
}

func (s *StdioConn) Read(p []byte) (int, error) {
	n, err := s.receive.Read(p)
	return n, err
}

func (s *StdioConn) Write(p []byte) (int, error) {
	n, err := s.send.Write(p)
	return n, err
}

// Close closes the stream to the child process.
func (s *StdioConn) Close() (err error) {
	s.closeSend.Do(func() {
		debug.Log("close stdio send connection")
		err = s.send.Close()
	})

	return err
}

// CloseAll closes both streams.
func (s *StdioConn) CloseAll() (err error) {
	err = s.Close()

	s.closeRecv.Do(func() {
		debug.Log("close stdio receive connection")
		err2 := s.receive.Close()
		if err == nil {
			err = err2
		}
	})

	return err
}

// LocalAddr returns nil.
func (s *StdioConn) LocalAddr() net.Addr {
	return Addr{}
}

// RemoteAddr returns nil.
func (s *StdioConn) RemoteAddr() net.Addr {
	return Addr{}
}

// SetDeadline sets the read/write deadline.
func (s *StdioConn) SetDeadline(t time.Time) error {
	err1 := s.receive.SetReadDeadline(t)
	err2 := s.send.SetWriteDeadline(t)
	if err1 != nil {
		return err1
	}
	return err2
}

// SetReadDeadline sets the read/write deadline.
func (s *StdioConn) SetReadDeadline(t time.Time) error {
	return s.receive.SetReadDeadline(t)
}

// SetWriteDeadline sets the read/write deadline.
func (s *StdioConn) SetWriteDeadline(t time.Time) error {
	return s.send.SetWriteDeadline(t)
}

// make sure StdioConn implements net.Conn
var _ net.Conn = &StdioConn{}

// Addr implements net.Addr for stdin/stdout.
type Addr struct{}

// Network returns the network type as a string.
func (a Addr) Network() string {
	return "stdio"
}

func (a Addr) String() string {
	return "stdio"
}
