package rclone

import (
	"net"
	"os"
	"os/exec"
	"sync"

	"github.com/restic/restic/internal/debug"
)

// StdioConn implements a net.Conn via stdin/stdout.
type StdioConn struct {
	stdin                   *os.File
	stdout                  *os.File
	bytesWritten, bytesRead int
	cmd                     *exec.Cmd
	close                   sync.Once
}

func (s *StdioConn) Read(p []byte) (int, error) {
	n, err := s.stdin.Read(p)
	s.bytesRead += n
	return n, err
}

func (s *StdioConn) Write(p []byte) (int, error) {
	n, err := s.stdout.Write(p)
	s.bytesWritten += n
	return n, err
}

// Close closes both streams.
func (s *StdioConn) Close() (err error) {
	s.close.Do(func() {
		debug.Log("close stdio connection")
		var errs []error

		for _, f := range []func() error{s.stdin.Close, s.stdout.Close} {
			err := f()
			if err != nil {
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			err = errs[0]
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
