package repository

import (
	"io"

	"github.com/restic/restic/crypto"
)

// decryptReadCloser couples an underlying reader with a DecryptReader and
// implements io.ReadCloser. On Close(), both readers are closed.
type decryptReadCloser struct {
	r  io.ReadCloser
	dr io.ReadCloser
}

func newDecryptReadCloser(key *crypto.Key, rd io.ReadCloser) (io.ReadCloser, error) {
	dr, err := crypto.DecryptFrom(key, rd)
	if err != nil {
		return nil, err
	}

	return &decryptReadCloser{r: rd, dr: dr}, nil
}

func (dr *decryptReadCloser) Read(buf []byte) (int, error) {
	return dr.dr.Read(buf)
}

func (dr *decryptReadCloser) Close() error {
	err := dr.dr.Close()
	if err != nil {
		return err
	}

	return dr.r.Close()
}
