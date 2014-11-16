package khepri

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"sync"

	"github.com/fd0/khepri/backend"
	"github.com/fd0/khepri/chunker"
)

type ContentHandler struct {
	be  backend.Server
	key *Key

	m       sync.Mutex
	content *StorageMap
}

// NewContentHandler creates a new content handler.
func NewContentHandler(be backend.Server, key *Key) (*ContentHandler, error) {
	ch := &ContentHandler{
		be:      be,
		key:     key,
		content: NewStorageMap(),
	}

	return ch, nil
}

// LoadSnapshot adds all blobs from a snapshot into the content handler and returns the snapshot.
func (ch *ContentHandler) LoadSnapshot(id backend.ID) (*Snapshot, error) {
	sn, err := LoadSnapshot(ch, id)
	if err != nil {
		return nil, err
	}

	ch.m.Lock()
	defer ch.m.Unlock()
	ch.content.Merge(sn.StorageMap)
	return sn, nil
}

// LoadAllSnapshots adds all blobs from all snapshots that can be decrypted
// into the content handler.
func (ch *ContentHandler) LoadAllSnapshots() error {
	// add all maps from all snapshots that can be decrypted to the storage map
	err := backend.EachID(ch.be, backend.Snapshot, func(id backend.ID) {
		sn, err := LoadSnapshot(ch, id)
		if err != nil {
			return
		}

		ch.m.Lock()
		defer ch.m.Unlock()
		ch.content.Merge(sn.StorageMap)
	})
	if err != nil {
		return err
	}

	return nil
}

// Save encrypts data and stores it to the backend as type t. If the data was
// already saved before, the blob is returned.
func (ch *ContentHandler) Save(t backend.Type, data []byte) (*Blob, error) {
	// compute plaintext hash
	id := backend.Hash(data)

	// test if the hash is already in the backend
	ch.m.Lock()
	defer ch.m.Unlock()
	blob := ch.content.Find(id)
	if blob != nil {
		return blob, nil
	}

	// else create a new blob
	blob = &Blob{
		ID:   id,
		Size: uint64(len(data)),
	}

	// encrypt blob
	ciphertext, err := ch.key.Encrypt(data)
	if err != nil {
		return nil, err
	}

	// save blob
	sid, err := ch.be.Create(t, ciphertext)
	if err != nil {
		return nil, err
	}

	blob.Storage = sid
	blob.StorageSize = uint64(len(ciphertext))

	// insert blob into the storage map
	ch.content.Insert(blob)

	return blob, nil
}

// SaveJSON serialises item as JSON and uses Save() to store it to the backend as type t.
func (ch *ContentHandler) SaveJSON(t backend.Type, item interface{}) (*Blob, error) {
	// convert to json
	data, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}

	// compress and save data
	return ch.Save(t, backend.Compress(data))
}

// SaveFile stores the content of the file on the backend as a Blob by calling
// Save for each chunk.
func (ch *ContentHandler) SaveFile(filename string, size uint) (Blobs, error) {
	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		return nil, err
	}

	// if the file is small enough, store it directly
	if size < chunker.MinSize {
		buf, err := ioutil.ReadAll(file)
		if err != nil {
			return nil, err
		}

		blob, err := ch.Save(backend.Data, buf)
		if err != nil {
			return nil, err
		}

		return Blobs{blob}, nil
	}

	// else store all chunks
	blobs := Blobs{}
	chunker := chunker.New(file)

	for {
		chunk, err := chunker.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		blob, err := ch.Save(backend.Data, chunk.Data)
		if err != nil {
			return nil, err
		}

		blobs = append(blobs, blob)
	}

	return blobs, nil
}

// Load tries to load and decrypt content identified by t and id from the backend.
func (ch *ContentHandler) Load(t backend.Type, id backend.ID) ([]byte, error) {
	if t == backend.Snapshot {
		// load data
		buf, err := ch.be.Get(t, id)
		if err != nil {
			return nil, err
		}

		// decrypt
		buf, err = ch.key.Decrypt(buf)
		if err != nil {
			return nil, err
		}

		return buf, nil
	}

	// lookup storage hash
	ch.m.Lock()
	defer ch.m.Unlock()
	blob := ch.content.Find(id)
	if blob == nil {
		return nil, errors.New("Storage ID not found")
	}

	// load data
	buf, err := ch.be.Get(t, blob.Storage)
	if err != nil {
		return nil, err
	}

	// check length
	if len(buf) != int(blob.StorageSize) {
		return nil, errors.New("Invalid storage length")
	}

	// decrypt
	buf, err = ch.key.Decrypt(buf)
	if err != nil {
		return nil, err
	}

	// check length
	if len(buf) != int(blob.Size) {
		return nil, errors.New("Invalid length")
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
	buf, err := ch.be.Get(t, id)
	if err != nil {
		return err
	}

	// decrypt
	buf, err = ch.key.Decrypt(buf)
	if err != nil {
		return err
	}

	// inflate and unmarshal
	err = json.Unmarshal(backend.Uncompress(buf), item)
	return err
}
