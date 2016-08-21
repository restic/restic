package backend

import (
	"crypto/rand"
	"io"
)

// RandomID retuns a randomly generated ID. This is mainly used for testing.
// When reading from rand fails, the function panics.
func RandomID() ID {
	id := ID{}
	_, err := io.ReadFull(rand.Reader, id[:])
	if err != nil {
		panic(err)
	}
	return id
}
