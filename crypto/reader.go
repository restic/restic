package crypto

import (
	"io"
	"io/ioutil"
)

type decryptReader struct {
	buf []byte
	pos int
}

func (d *decryptReader) Read(dst []byte) (int, error) {
	if d.buf == nil {
		return 0, io.EOF
	}

	if len(dst) == 0 {
		return 0, nil
	}

	remaining := len(d.buf) - d.pos
	if len(dst) >= remaining {
		n := copy(dst, d.buf[d.pos:])
		d.Close()
		return n, io.EOF
	}

	n := copy(dst, d.buf[d.pos:d.pos+len(dst)])
	d.pos += n

	return n, nil
}

func (d *decryptReader) ReadByte() (c byte, err error) {
	if d.buf == nil {
		return 0, io.EOF
	}

	remaining := len(d.buf) - d.pos
	if remaining == 1 {
		c = d.buf[d.pos]
		d.Close()
		return c, io.EOF
	}

	c = d.buf[d.pos]
	d.pos++

	return
}

func (d *decryptReader) Close() error {
	if d.buf == nil {
		return nil
	}

	freeBuffer(d.buf)
	d.buf = nil
	return nil
}

// DecryptFrom verifies and decrypts the ciphertext read from rd with ks and
// makes it available on the returned Reader. Ciphertext must be in the form IV
// || Ciphertext || MAC. In order to correctly verify the ciphertext, rd is
// drained, locally buffered and made available on the returned Reader
// afterwards. If a MAC verification failure is observed, it is returned
// immediately.
func DecryptFrom(ks *Key, rd io.Reader) (io.ReadCloser, error) {
	ciphertext := getBuffer()

	ciphertext = ciphertext[0:cap(ciphertext)]
	n, err := io.ReadFull(rd, ciphertext)
	if err != io.ErrUnexpectedEOF {
		// read remaining data
		buf, e := ioutil.ReadAll(rd)
		ciphertext = append(ciphertext, buf...)
		n += len(buf)
		err = e
	} else {
		err = nil
	}

	if err != nil {
		return nil, err
	}

	ciphertext = ciphertext[:n]

	// decrypt
	ciphertext, err = Decrypt(ks, ciphertext, ciphertext)
	if err != nil {
		return nil, err
	}

	return &decryptReader{buf: ciphertext}, nil
}
