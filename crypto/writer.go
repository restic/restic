package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"sync"
)

type encryptWriter struct {
	iv      iv
	wroteIV bool
	data    *bytes.Buffer
	key     *Key
	s       cipher.Stream
	w       io.Writer
	origWr  io.Writer
	err     error // remember error writing iv
}

func (e *encryptWriter) Close() error {
	// write mac
	mac := poly1305Sign(e.data.Bytes()[ivSize:], e.data.Bytes()[:ivSize], &e.key.Sign)
	_, err := e.origWr.Write(mac)
	if err != nil {
		return err
	}

	// return buffer
	bufPool.Put(e.data.Bytes())

	return nil
}

const encryptWriterChunkSize = 512 * 1024 // 512 KiB
var encryptWriterBufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, encryptWriterChunkSize)
	},
}

func (e *encryptWriter) Write(p []byte) (int, error) {
	// write iv first
	if !e.wroteIV {
		_, e.err = e.origWr.Write(e.iv[:])
		e.wroteIV = true
	}

	if e.err != nil {
		return 0, e.err
	}

	buf := encryptWriterBufPool.Get().([]byte)
	defer encryptWriterBufPool.Put(buf)

	written := 0
	for len(p) > 0 {
		max := len(p)
		if max > encryptWriterChunkSize {
			max = encryptWriterChunkSize
		}

		e.s.XORKeyStream(buf, p[:max])
		n, err := e.w.Write(buf[:max])
		if n != max {
			if err == nil { // should never happen
				err = io.ErrShortWrite
			}
		}

		written += n
		p = p[n:]

		if err != nil {
			e.err = err
			return written, err
		}
	}

	return written, nil
}

// EncryptTo buffers data written to the returned io.WriteCloser. When Close()
// is called, the data is encrypted an written to the underlying writer.
func EncryptTo(ks *Key, wr io.Writer) io.WriteCloser {
	ew := &encryptWriter{
		iv:     newIV(),
		data:   bytes.NewBuffer(getBuffer()[:0]),
		key:    ks,
		origWr: wr,
	}

	// buffer iv for mac
	_, err := ew.data.Write(ew.iv[:])
	if err != nil {
		panic(err)
	}

	c, err := aes.NewCipher(ks.Encrypt[:])
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	ew.s = cipher.NewCTR(c, ew.iv[:])
	ew.w = io.MultiWriter(ew.data, wr)

	return ew
}
