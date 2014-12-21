package restic

import (
	"errors"

	"github.com/restic/restic/backend"
)

type Server struct {
	be  backend.Backend
	key *Key
}

func NewServer(be backend.Backend) Server {
	return Server{be: be}
}

func NewServerWithKey(be backend.Backend, key *Key) Server {
	return Server{be: be, key: key}
}

// Each lists all entries of type t in the backend and calls function f() with
// the id and data.
func (s Server) Each(t backend.Type, f func(id backend.ID, data []byte, err error)) error {
	return backend.Each(s.be, t, f)
}

// Each lists all entries of type t in the backend and calls function f() with
// the id.
func (s Server) EachID(t backend.Type, f func(backend.ID)) error {
	return backend.EachID(s.be, t, f)
}

// Find loads the list of all blobs of type t and searches for IDs which start
// with prefix. If none is found, nil and ErrNoIDPrefixFound is returned. If
// more than one is found, nil and ErrMultipleIDMatches is returned.
func (s Server) Find(t backend.Type, prefix string) (backend.ID, error) {
	return backend.Find(s.be, t, prefix)
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func (s Server) FindSnapshot(id string) (backend.ID, error) {
	return backend.FindSnapshot(s.be, id)
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func (s Server) PrefixLength(t backend.Type) (int, error) {
	return backend.PrefixLength(s.be, t)
}

// Returns the backend used for this server.
func (s Server) Backend() backend.Backend {
	return s.be
}

// Each calls Each() with the given parameters, Decrypt() on the ciphertext
// and, on successful decryption, f with the plaintext.
func (s Server) EachDecrypted(t backend.Type, f func(backend.ID, []byte, error)) error {
	if s.key == nil {
		return errors.New("key for server not set")
	}

	return s.Each(t, func(id backend.ID, data []byte, e error) {
		if e != nil {
			f(id, nil, e)
			return
		}

		buf, err := s.key.Decrypt(data)
		if err != nil {
			f(id, nil, err)
			return
		}

		f(id, buf, nil)
	})
}

// Proxy methods to backend

func (s Server) List(t backend.Type) (backend.IDs, error) {
	return s.be.List(t)
}

func (s Server) Get(t backend.Type, id backend.ID) ([]byte, error) {
	return s.be.Get(t, id)
}

func (s Server) Create(t backend.Type, data []byte) (backend.ID, error) {
	return s.be.Create(t, data)
}

func (s Server) Test(t backend.Type, id backend.ID) (bool, error) {
	return s.be.Test(t, id)
}

func (s Server) Remove(t backend.Type, id backend.ID) error {
	return s.be.Remove(t, id)
}

func (s Server) Close() error {
	return s.be.Close()
}

func (s Server) Delete() error {
	if b, ok := s.be.(backend.Deleter); ok {
		return b.Delete()
	}

	return errors.New("Delete() called for backend that does not implement this method")
}
