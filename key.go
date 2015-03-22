package restic

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"time"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"

	"golang.org/x/crypto/poly1305"
)

// max size is 8MiB, defined in chunker
const macSize = poly1305.TagSize // Poly1305 size is 16 byte
const maxCiphertextSize = ivSize + chunker.MaxSize + macSize
const CiphertextExtension = ivSize + macSize

var (
	// ErrUnauthenticated is returned when ciphertext verification has failed.
	ErrUnauthenticated = errors.New("ciphertext verification failed")
	// ErrNoKeyFound is returned when no key for the repository could be decrypted.
	ErrNoKeyFound = errors.New("no key could be found")
	// ErrBufferTooSmall is returned when the destination slice is too small
	// for the ciphertext.
	ErrBufferTooSmall = errors.New("destination buffer too small")
)

// TODO: figure out scrypt values on the fly depending on the current
// hardware.
const (
	scryptN        = 65536
	scryptR        = 8
	scryptP        = 1
	scryptSaltsize = 64
)

// Key represents an encrypted master key for a repository.
type Key struct {
	Created  time.Time `json:"created"`
	Username string    `json:"username"`
	Hostname string    `json:"hostname"`
	Comment  string    `json:"comment,omitempty"`

	KDF  string `json:"kdf"`
	N    int    `json:"N"`
	R    int    `json:"r"`
	P    int    `json:"p"`
	Salt []byte `json:"salt"`
	Data []byte `json:"data"`

	user   *keys
	master *keys

	id backend.ID
}

// MasterKeys holds signing and encryption keys for a repository. It is stored
// encrypted and signed as a JSON data structure in the Data field of the Key
// structure.
type keys struct {
	Sign    MACKey
	Encrypt AESKey
}

// CreateKey initializes a master key in the given backend and encrypts it with
// the password.
func CreateKey(s Server, password string) (*Key, error) {
	return AddKey(s, password, nil)
}

// OpenKey tries do decrypt the key specified by id with the given password.
func OpenKey(s Server, id backend.ID, password string) (*Key, error) {
	k, err := LoadKey(s, id)
	if err != nil {
		return nil, err
	}

	// check KDF
	if k.KDF != "scrypt" {
		return nil, errors.New("only supported KDF is scrypt()")
	}

	// derive user key
	k.user, err = kdf(k, password)
	if err != nil {
		return nil, err
	}

	// decrypt master keys
	buf, err := k.DecryptUser([]byte{}, k.Data)
	if err != nil {
		return nil, err
	}

	// restore json
	k.master = &keys{}
	err = json.Unmarshal(buf, k.master)
	if err != nil {
		return nil, err
	}
	k.id = id

	return k, nil
}

// SearchKey tries to decrypt all keys in the backend with the given password.
// If none could be found, ErrNoKeyFound is returned.
func SearchKey(s Server, password string) (*Key, error) {
	// list all keys
	ids, err := s.List(backend.Key)
	if err != nil {
		panic(err)
	}

	// try all keys in repo
	var key *Key
	for _, id := range ids {
		key, err = OpenKey(s, id, password)
		if err != nil {
			continue
		}

		return key, nil
	}

	return nil, ErrNoKeyFound
}

// LoadKey loads a key from the backend.
func LoadKey(s Server, id backend.ID) (*Key, error) {
	// extract data from repo
	data, err := s.Get(backend.Key, id)
	if err != nil {
		return nil, err
	}

	// restore json
	k := &Key{}
	err = json.Unmarshal(data, k)
	if err != nil {
		return nil, err
	}

	return k, err
}

// AddKey adds a new key to an already existing repository.
func AddKey(s Server, password string, template *Key) (*Key, error) {
	// fill meta data about key
	newkey := &Key{
		Created: time.Now(),
		KDF:     "scrypt",
		N:       scryptN,
		R:       scryptR,
		P:       scryptP,
	}

	hn, err := os.Hostname()
	if err == nil {
		newkey.Hostname = hn
	}

	usr, err := user.Current()
	if err == nil {
		newkey.Username = usr.Username
	}

	// generate random salt
	newkey.Salt = make([]byte, scryptSaltsize)
	n, err := rand.Read(newkey.Salt)
	if n != scryptSaltsize || err != nil {
		panic("unable to read enough random bytes for salt")
	}

	// call KDF to derive user key
	newkey.user, err = kdf(newkey, password)
	if err != nil {
		return nil, err
	}

	if template == nil {
		// generate new random master keys
		newkey.master = generateRandomKeys()
	} else {
		// copy master keys from old key
		newkey.master = template.master
	}

	// encrypt master keys (as json) with user key
	buf, err := json.Marshal(newkey.master)
	if err != nil {
		return nil, err
	}

	newkey.Data = GetChunkBuf("key")
	n, err = newkey.EncryptUser(newkey.Data, buf)
	newkey.Data = newkey.Data[:n]

	// dump as json
	buf, err = json.Marshal(newkey)
	if err != nil {
		return nil, err
	}

	// store in repository and return
	blob, err := s.Create(backend.Key)
	if err != nil {
		return nil, err
	}

	_, err = blob.Write(buf)
	if err != nil {
		return nil, err
	}

	err = blob.Close()
	if err != nil {
		return nil, err
	}

	id, err := blob.ID()
	if err != nil {
		return nil, err
	}

	newkey.id = id

	FreeChunkBuf("key", newkey.Data)

	return newkey, nil
}

func (k *Key) newIV(buf []byte) error {
	_, err := io.ReadFull(rand.Reader, buf[:ivSize])
	buf = buf[:ivSize]
	if err != nil {
		return err
	}

	return nil
}

// EncryptUser encrypts and signs data with the user key. Stored in ciphertext
// is IV || Ciphertext || MAC.
func (k *Key) EncryptUser(ciphertext, plaintext []byte) (int, error) {
	return Encrypt(k.user, ciphertext, plaintext)
}

// Encrypt encrypts and signs data with the master key. Stored in ciphertext is
// IV || Ciphertext || MAC. Returns the ciphertext length.
func (k *Key) Encrypt(ciphertext, plaintext []byte) (int, error) {
	return Encrypt(k.master, ciphertext, plaintext)
}

// EncryptTo encrypts and signs data with the master key. The returned
// io.Writer writes IV || Ciphertext || HMAC. For the hash function, SHA256 is
// used.
func (k *Key) EncryptTo(wr io.Writer) io.WriteCloser {
	return EncryptTo(k.master, wr)
}

// EncryptUserTo encrypts and signs data with the user key. The returned
// io.Writer writes IV || Ciphertext || HMAC. For the hash function, SHA256 is
// used.
func (k *Key) EncryptUserTo(wr io.Writer) io.WriteCloser {
	return EncryptTo(k.user, wr)
}

// Decrypt verifes and decrypts the ciphertext with the master key. Ciphertext
// must be in the form IV || Ciphertext || MAC.
func (k *Key) Decrypt(plaintext, ciphertext []byte) ([]byte, error) {
	return Decrypt(k.master, plaintext, ciphertext)
}

// DecryptUser verifes and decrypts the ciphertext with the user key. Ciphertext
// must be in the form IV || Ciphertext || MAC.
func (k *Key) DecryptUser(plaintext, ciphertext []byte) ([]byte, error) {
	return Decrypt(k.user, plaintext, ciphertext)
}

// DecryptFrom verifies and decrypts the ciphertext read from rd and makes it
// available on the returned Reader. Ciphertext must be in the form IV ||
// Ciphertext || MAC. In order to correctly verify the ciphertext, rd is
// drained, locally buffered and made available on the returned Reader
// afterwards. If an MAC verification failure is observed, it is returned
// immediately.
func (k *Key) DecryptFrom(rd io.Reader) (io.ReadCloser, error) {
	return DecryptFrom(k.master, rd)
}

// DecryptFrom verifies and decrypts the ciphertext read from rd with the user
// key and makes it available on the returned Reader. Ciphertext must be in the
// form IV || Ciphertext || MAC. In order to correctly verify the ciphertext,
// rd is drained, locally buffered and made available on the returned Reader
// afterwards. If an MAC verification failure is observed, it is returned
// immediately.
func (k *Key) DecryptUserFrom(rd io.Reader) (io.ReadCloser, error) {
	return DecryptFrom(k.user, rd)
}

func (k *Key) String() string {
	if k == nil {
		return "<Key nil>"
	}
	return fmt.Sprintf("<Key of %s@%s, created on %s>", k.Username, k.Hostname, k.Created)
}

func (k Key) ID() backend.ID {
	return k.id
}
