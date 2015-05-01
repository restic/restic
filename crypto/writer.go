package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
)

type encryptWriter struct {
	data   []byte
	key    *Key
	s      cipher.Stream
	w      io.Writer
	closed bool
}

func (e *encryptWriter) Close() error {
	if e == nil {
		return nil
	}

	if e.closed {
		return errors.New("Close() called on already closed writer")
	}
	e.closed = true

	// encrypt everything
	iv, c := e.data[:ivSize], e.data[ivSize:]
	e.s.XORKeyStream(c, c)

	// compute mac
	mac := poly1305MAC(c, iv, &e.key.MAC)
	e.data = append(e.data, mac...)

	// write everything
	n, err := e.w.Write(e.data)
	if err != nil {
		return err
	}

	if n != len(e.data) {
		return errors.New("not all bytes written")
	}

	// return buffer to pool
	freeBuffer(e.data)

	return nil
}

func (e *encryptWriter) Write(p []byte) (int, error) {
	// if e.data is too small, return it to the buffer and create new slice
	if cap(e.data) < len(e.data)+len(p) {
		b := make([]byte, len(e.data), len(e.data)*2)
		copy(b, e.data)
		freeBuffer(e.data)
		e.data = b
	}

	// copy new data to e.data
	e.data = append(e.data, p...)
	return len(p), nil
}

// EncryptTo buffers data written to the returned io.WriteCloser. When Close()
// is called, the data is encrypted and written to the underlying writer.
func EncryptTo(ks *Key, wr io.Writer) io.WriteCloser {
	ew := &encryptWriter{
		data: getBuffer(),
		key:  ks,
	}

	// buffer iv for mac
	ew.data = ew.data[:ivSize]
	copy(ew.data, newIV())

	c, err := aes.NewCipher(ks.Encrypt[:])
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	ew.s = cipher.NewCTR(c, ew.data[:ivSize])
	ew.w = wr

	return ew
}
