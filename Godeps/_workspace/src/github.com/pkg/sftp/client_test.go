package sftp

import (
	"io"
	"os"
	"testing"

	"github.com/kr/fs"
)

// assert that *Client implements fs.FileSystem
var _ fs.FileSystem = new(Client)

// assert that *File implements io.ReadWriteCloser
var _ io.ReadWriteCloser = new(File)

var ok = &StatusError{Code: ssh_FX_OK}
var eof = &StatusError{Code: ssh_FX_EOF}
var fail = &StatusError{Code: ssh_FX_FAILURE}

var eofOrErrTests = []struct {
	err, want error
}{
	{nil, nil},
	{eof, io.EOF},
	{ok, ok},
	{io.EOF, io.EOF},
}

func TestEofOrErr(t *testing.T) {
	for _, tt := range eofOrErrTests {
		got := eofOrErr(tt.err)
		if got != tt.want {
			t.Errorf("eofOrErr(%#v): want: %#v, got: %#v", tt.err, tt.want, got)
		}
	}
}

var okOrErrTests = []struct {
	err, want error
}{
	{nil, nil},
	{eof, eof},
	{ok, nil},
	{io.EOF, io.EOF},
}

func TestOkOrErr(t *testing.T) {
	for _, tt := range okOrErrTests {
		got := okOrErr(tt.err)
		if got != tt.want {
			t.Errorf("okOrErr(%#v): want: %#v, got: %#v", tt.err, tt.want, got)
		}
	}
}

var flagsTests = []struct {
	flags int
	want  uint32
}{
	{os.O_RDONLY, ssh_FXF_READ},
	{os.O_WRONLY, ssh_FXF_WRITE},
	{os.O_RDWR, ssh_FXF_READ | ssh_FXF_WRITE},
	{os.O_RDWR | os.O_CREATE | os.O_TRUNC, ssh_FXF_READ | ssh_FXF_WRITE | ssh_FXF_CREAT | ssh_FXF_TRUNC},
	{os.O_WRONLY | os.O_APPEND, ssh_FXF_WRITE | ssh_FXF_APPEND},
}

func TestFlags(t *testing.T) {
	for i, tt := range flagsTests {
		got := flags(tt.flags)
		if got != tt.want {
			t.Errorf("test %v: flags(%x): want: %x, got: %x", i, tt.flags, tt.want, got)
		}
	}
}
