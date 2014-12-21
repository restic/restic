package restic

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/restic/restic/backend"
)

var ErrWrongData = errors.New("wrong data decrypt, checksum does not match")

type ContentHandler struct {
	s Server

	bl *BlobList
}

// NewContentHandler creates a new content handler.
func NewContentHandler(s Server) (*ContentHandler, error) {
	ch := &ContentHandler{
		s:  s,
		bl: NewBlobList(),
	}

	return ch, nil
}

// LoadSnapshot adds all blobs from a snapshot into the content handler and returns the snapshot.
func (ch *ContentHandler) LoadSnapshot(id backend.ID) (*Snapshot, error) {
	sn, err := LoadSnapshot(ch, id)
	if err != nil {
		return nil, err
	}

	sn.bl, err = LoadBlobList(ch, sn.Map)
	if err != nil {
		return nil, err
	}

	ch.bl.Merge(sn.bl)

	return sn, nil
}

// LoadAllMaps adds all blobs from all snapshots that can be decrypted
// into the content handler.
func (ch *ContentHandler) LoadAllMaps() error {
	// add all maps from all snapshots that can be decrypted to the storage map
	err := backend.EachID(ch.s, backend.Map, func(id backend.ID) {
		bl, err := LoadBlobList(ch, id)
		if err != nil {
			return
		}

		ch.bl.Merge(bl)
	})
	if err != nil {
		return err
	}

	return nil
}

// Save encrypts data and stores it to the backend as type t. If the data was
// already saved before, the blob is returned.
func (ch *ContentHandler) Save(t backend.Type, data []byte) (Blob, error) {
	// compute plaintext hash
	id := backend.Hash(data)

	// test if the hash is already in the backend
	blob, err := ch.bl.Find(Blob{ID: id})
	if err == nil {
		id.Free()
		return blob, nil
	}

	// else create a new blob
	blob = Blob{
		ID:   id,
		Size: uint64(len(data)),
	}

	var ciphertext []byte

	// for a bloblist/map, use a larger buffer
	if t == backend.Map {
		ciphertext = make([]byte, len(data)+CiphertextExtension)
	} else {
		// otherwise use buffer from pool
		ciphertext = GetChunkBuf("ch.Save()")
		defer FreeChunkBuf("ch.Save()", ciphertext)
	}

	// encrypt blob
	n, err := ch.s.Encrypt(ciphertext, data)
	if err != nil {
		return Blob{}, err
	}

	ciphertext = ciphertext[:n]

	// save blob
	sid, err := ch.s.Create(t, ciphertext)
	if err != nil {
		return Blob{}, err
	}

	blob.Storage = sid
	blob.StorageSize = uint64(len(ciphertext))

	// insert blob into the storage map
	ch.bl.Insert(blob)

	return blob, nil
}

// SaveJSON serialises item as JSON and uses Save() to store it to the backend as type t.
func (ch *ContentHandler) SaveJSON(t backend.Type, item interface{}) (Blob, error) {
	// convert to json
	data, err := json.Marshal(item)
	if err != nil {
		return Blob{}, err
	}

	// compress and save data
	return ch.Save(t, backend.Compress(data))
}

// Load tries to load and decrypt content identified by t and id from the backend.
func (ch *ContentHandler) Load(t backend.Type, id backend.ID) ([]byte, error) {
	if t == backend.Snapshot {
		// load data
		buf, err := ch.s.Get(t, id)
		if err != nil {
			return nil, err
		}

		// decrypt
		buf, err = ch.s.Decrypt(buf)
		if err != nil {
			return nil, err
		}

		return buf, nil
	}

	// lookup storage hash
	blob, err := ch.bl.Find(Blob{ID: id})
	if err != nil {
		return nil, fmt.Errorf("Storage ID %s not found", id)
	}

	// load data
	buf, err := ch.s.Get(t, blob.Storage)
	if err != nil {
		return nil, err
	}

	// check length
	if len(buf) != int(blob.StorageSize) {
		return nil, errors.New("Invalid storage length")
	}

	// decrypt
	buf, err = ch.s.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	// check length
	if len(buf) != int(blob.Size) {
		return nil, errors.New("Invalid length")
	}

	// check SHA256 sum
	if !id.Equal(backend.Hash(buf)) {
		return nil, ErrWrongData
	}

	return buf, nil
}

// LoadJSON calls Load() to get content from the backend and afterwards calls
// json.Unmarshal on the item.
func (ch *ContentHandler) LoadJSON(t backend.Type, id backend.ID, item interface{}) error {
	// load from backend
	buf, err := ch.Load(t, id)
	if err != nil {
		return err
	}

	// inflate and unmarshal
	err = json.Unmarshal(backend.Uncompress(buf), item)
	return err
}

// LoadJSONRaw loads data with the given storage id and type from the backend,
// decrypts it and calls json.Unmarshal on the item.
func (ch *ContentHandler) LoadJSONRaw(t backend.Type, id backend.ID, item interface{}) error {
	// load data
	buf, err := ch.s.Get(t, id)
	if err != nil {
		return err
	}

	// decrypt
	buf, err = ch.s.Decrypt(buf)
	if err != nil {
		return err
	}

	// inflate and unmarshal
	err = json.Unmarshal(backend.Uncompress(buf), item)
	return err
}
