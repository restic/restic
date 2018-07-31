package stdioconn

import (
	"net"
	"os"
	"sync"

	"github.com/restic/restic/internal/debug"
)

// StdioConn implements a net.Conn via os.File input/output.
type StdioConn struct {
	input  *os.File
	output *os.File
	close  sync.Once
}

// New creates a StdioConn using the provided pipes
func New(input *os.File, output *os.File) *StdioConn {
	return &StdioConn{
		input:  input,
		output: output,
	}
}

func (s *StdioConn) Read(p []byte) (int, error) {
	n, err := s.input.Read(p)
	return n, err
}

func (s *StdioConn) Write(p []byte) (int, error) {
	n, err := s.output.Write(p)
	return n, err
}

// Close closes both streams.
func (s *StdioConn) Close() (err error) {
	s.close.Do(func() {
		debug.Log("close stdio connection")
		var errs []error

		for _, f := range []func() error{s.input.Close, s.output.Close} {
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

// Addr implements net.Addr for StdioConn.
type Addr struct{}

// Network returns the network type as a string.
func (a Addr) Network() string {
	return "stdio"
}

func (a Addr) String() string {
	return "stdio"
}
