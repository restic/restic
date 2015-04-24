package crypto

import (
	"bytes"
	"errors"
	"io"
)

type decryptReader struct {
	buf []byte
	rd  *bytes.Reader
}

func (d *decryptReader) Read(dst []byte) (n int, err error) {
	if d.buf == nil {
		return 0, io.EOF
	}

	n, err = d.rd.Read(dst)
	if err == io.EOF {
		d.free()
	}

	return
}

func (d *decryptReader) free() {
	freeBuffer(d.buf)
	d.buf = nil
}

func (d *decryptReader) Close() error {
	if d.buf == nil {
		return nil
	}

	d.free()
	return nil
}

func (d *decryptReader) ReadByte() (c byte, err error) {
	if d.buf == nil {
		return 0, io.EOF
	}

	c, err = d.rd.ReadByte()
	if err == io.EOF {
		d.free()
	}

	return
}

func (d *decryptReader) WriteTo(w io.Writer) (n int64, err error) {
	if d.buf == nil {
		return 0, errors.New("WriteTo() called on drained reader")
	}

	n, err = d.rd.WriteTo(w)
	d.free()

	return
}

// DecryptFrom verifies and decrypts the ciphertext read from rd with ks and
// makes it available on the returned Reader. Ciphertext must be in the form IV
// || Ciphertext || MAC. In order to correctly verify the ciphertext, rd is
// drained, locally buffered and made available on the returned Reader
// afterwards. If a MAC verification failure is observed, it is returned
// immediately.
func DecryptFrom(ks *Key, rd io.Reader) (io.ReadCloser, error) {
	buf := bytes.NewBuffer(getBuffer()[:0])
	_, err := buf.ReadFrom(rd)
	if err != nil {
		return nil, err
	}

	ciphertext := buf.Bytes()

	// decrypt
	ciphertext, err = Decrypt(ks, ciphertext, ciphertext)
	if err != nil {
		return nil, err
	}

	return &decryptReader{buf: ciphertext, rd: bytes.NewReader(ciphertext)}, nil
}
