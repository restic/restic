package repository

import (
	"bytes"
	"encoding/json"
	"restic"
	"restic/backend"
	"restic/crypto"
	"restic/debug"
	"restic/errors"
)

// Load loads and decrypts data identified by h from the backend.
func Load(be restic.Backend, key *crypto.Key, t restic.FileType, id restic.ID) ([]byte, error) {
	h := restic.Handle{Type: t, Name: id.String()}
	buf, err := backend.LoadAll(be, h)
	if err != nil {
		debug.Log("error loading %v: %v", h, err)
		return nil, err
	}

	if t != restic.ConfigFile && !restic.Hash(buf).Equal(id) {
		return nil, errors.Errorf("load %v: invalid data returned", h)
	}

	// decrypt
	n, err := crypto.Decrypt(key, buf, buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

// SaveJSON serialises item as JSON and encrypts and saves it in the backend as
// type t. It returns the storage hash.
func SaveJSON(be restic.Backend, key *crypto.Key, t restic.FileType, item interface{}) (id restic.ID, err error) {
	buf, err := json.Marshal(item)
	if err != nil {
		return id, errors.Wrap(err, "Marshal")
	}

	return Save(be, key, t, buf)
}

// Save encrypts data and stores it in the backend. Returned is the storage
// hash.
func Save(be restic.Backend, key *crypto.Key, t restic.FileType, buf []byte) (id restic.ID, err error) {
	ciphertext := restic.NewBlobBuffer(len(buf))
	ciphertext, err = crypto.Encrypt(key, ciphertext, buf)
	if err != nil {
		return id, err
	}

	id = restic.Hash(ciphertext)
	h := restic.Handle{Type: t, Name: id.String()}

	err = be.Save(h, bytes.NewReader(ciphertext))
	if err != nil {
		return id, err
	}

	return id, nil
}
