// +build go1.10

package stdioconn

import "time"

// SetDeadline sets the read/write deadline.
func (s *StdioConn) SetDeadline(t time.Time) error {
	err1 := s.input.SetReadDeadline(t)
	err2 := s.output.SetWriteDeadline(t)
	if err1 != nil {
		return err1
	}
	return err2
}

// SetReadDeadline sets the read/write deadline.
func (s *StdioConn) SetReadDeadline(t time.Time) error {
	return s.input.SetReadDeadline(t)
}

// SetWriteDeadline sets the read/write deadline.
func (s *StdioConn) SetWriteDeadline(t time.Time) error {
	return s.output.SetWriteDeadline(t)
}
