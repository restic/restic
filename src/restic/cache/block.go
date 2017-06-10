package cache

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"restic"
	"restic/crypto"
	"restic/errors"
)

// BlockWriter adds new blocks to an open cache file.
type BlockWriter struct {
	*os.File
	key *crypto.Key
}

// NewBlockWriter creates a new file and opens a BlockWriter.
func NewBlockWriter(name string, key *crypto.Key) (*BlockWriter, error) {
	f, err := os.Create(name)
	if err != nil {
		return nil, errors.Wrap(err, "Create")
	}

	return &BlockWriter{
		File: f,
		key:  key,
	}, nil
}

// Write encrypts and writes a new block to the file.
func (b *BlockWriter) Write(block []byte) error {
	buf := make([]byte, restic.CiphertextLength(len(block)))
	buf, err := crypto.Encrypt(b.key, buf, block)
	if err != nil {
		return err
	}

	err = binary.Write(b.File, binary.LittleEndian, uint32(len(buf)))
	if err != nil {
		return errors.Wrap(err, "binary.Write")
	}

	_, err = b.File.Write(buf)
	return errors.Wrap(err, "Write")
}

// WriteJSON serialises item, encrypts the result and adds it as a new blob.
func (b *BlockWriter) WriteJSON(item interface{}) error {
	buf, err := json.Marshal(item)
	if err != nil {
		return errors.Wrap(err, "Marshal")
	}

	return errors.Wrap(b.Write(buf), "Write")
}

// BlockReader reads and decrypts blocks from a cache file.
type BlockReader struct {
	*os.File
	key *crypto.Key
}

// NewBlockReader opens a file and returns a BlockReader.
func NewBlockReader(name string, key *crypto.Key) (*BlockReader, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, errors.Wrap(err, "Open")
	}

	return &BlockReader{
		File: f,
		key:  key,
	}, nil
}

// Read reads and decrypts the next block from the file. The buffer is enlarged
// if necessary
func (b *BlockReader) Read(buf []byte) ([]byte, error) {
	// read length
	var l uint32
	err := binary.Read(b.File, binary.LittleEndian, &l)
	if err != nil {
		return nil, err
	}
	blockLen := int(l)

	// make sure buf is large enough
	if len(buf) < blockLen {
		buf = make([]byte, blockLen)
	}

	buf = buf[:blockLen]

	_, err = io.ReadFull(b.File, buf)
	if err != nil {
		return nil, err
	}

	n, err := crypto.Decrypt(b.key, buf, buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

// ReadJSON loads a block and unserialises it into item after decryption.
func (b *BlockReader) ReadJSON(item interface{}) error {
	buf, err := b.Read(nil)
	if err != nil {
		return err
	}

	return errors.Wrap(json.Unmarshal(buf, item), "Unmarshal")
}
