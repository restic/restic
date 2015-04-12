package restic

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"time"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/debug"
)

var (
	// ErrNoKeyFound is returned when no key for the repository could be decrypted.
	ErrNoKeyFound = errors.New("no key could be found")
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

	KDF  string `json:"kdf"`
	N    int    `json:"N"`
	R    int    `json:"r"`
	P    int    `json:"p"`
	Salt []byte `json:"salt"`
	Data []byte `json:"data"`

	user   *crypto.Key
	master *crypto.Key

	name string
}

// CreateKey initializes a master key in the given backend and encrypts it with
// the password.
func CreateKey(s Server, password string) (*Key, error) {
	return AddKey(s, password, nil)
}

// OpenKey tries do decrypt the key specified by name with the given password.
func OpenKey(s Server, name string, password string) (*Key, error) {
	k, err := LoadKey(s, name)
	if err != nil {
		return nil, err
	}

	// check KDF
	if k.KDF != "scrypt" {
		return nil, errors.New("only supported KDF is scrypt()")
	}

	// derive user key
	k.user, err = crypto.KDF(k.N, k.R, k.P, k.Salt, password)
	if err != nil {
		return nil, err
	}

	// decrypt master keys
	buf, err := crypto.Decrypt(k.user, []byte{}, k.Data)
	if err != nil {
		return nil, err
	}

	// restore json
	k.master = &crypto.Key{}
	err = json.Unmarshal(buf, k.master)
	if err != nil {
		return nil, err
	}
	k.name = name

	// test if polynomial is valid and irreducible
	if k.master.ChunkerPolynomial == 0 {
		return nil, errors.New("Polynomial for content defined chunking is zero")
	}

	if !k.master.ChunkerPolynomial.Irreducible() {
		return nil, errors.New("Polynomial for content defined chunking is invalid")
	}

	debug.Log("OpenKey", "Master keys loaded, polynomial %v", k.master.ChunkerPolynomial)

	return k, nil
}

// SearchKey tries to decrypt all keys in the backend with the given password.
// If none could be found, ErrNoKeyFound is returned.
func SearchKey(s Server, password string) (*Key, error) {
	// try all keys in repo
	done := make(chan struct{})
	defer close(done)
	for name := range s.List(backend.Key, done) {
		key, err := OpenKey(s, name, password)
		if err != nil {
			continue
		}

		return key, nil
	}

	return nil, ErrNoKeyFound
}

// LoadKey loads a key from the backend.
func LoadKey(s Server, name string) (*Key, error) {
	// extract data from repo
	rd, err := s.be.Get(backend.Key, name)
	if err != nil {
		return nil, err
	}
	defer rd.Close()

	// restore json
	dec := json.NewDecoder(rd)
	k := Key{}
	err = dec.Decode(&k)
	if err != nil {
		return nil, err
	}

	return &k, nil
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
	newkey.user, err = crypto.KDF(newkey.N, newkey.R, newkey.P, newkey.Salt, password)
	if err != nil {
		return nil, err
	}

	if template == nil {
		// generate new random master keys
		newkey.master = crypto.NewKey()
		// generate random polynomial for cdc
		p, err := chunker.RandomPolynomial()
		if err != nil {
			debug.Log("AddKey", "error generating new polynomial for cdc: %v", err)
			return nil, err
		}
		debug.Log("AddKey", "generated new polynomial for cdc: %v", p)
		newkey.master.ChunkerPolynomial = p
	} else {
		// copy master keys from old key
		newkey.master = template.master
	}

	// encrypt master keys (as json) with user key
	buf, err := json.Marshal(newkey.master)
	if err != nil {
		return nil, err
	}

	newkey.Data, err = crypto.Encrypt(newkey.user, GetChunkBuf("key"), buf)

	// dump as json
	buf, err = json.Marshal(newkey)
	if err != nil {
		return nil, err
	}

	// store in repository and return
	blob, err := s.be.Create()
	if err != nil {
		return nil, err
	}

	plainhw := backend.NewHashingWriter(blob, sha256.New())

	_, err = plainhw.Write(buf)
	if err != nil {
		return nil, err
	}

	name := backend.ID(plainhw.Sum(nil)).String()

	err = blob.Finalize(backend.Key, name)
	if err != nil {
		return nil, err
	}

	newkey.name = name

	FreeChunkBuf("key", newkey.Data)

	return newkey, nil
}

// Encrypt encrypts and signs data with the master key. Stored in ciphertext is
// IV || Ciphertext || MAC. Returns the ciphertext, which is extended if
// necessary.
func (k *Key) Encrypt(ciphertext, plaintext []byte) ([]byte, error) {
	return crypto.Encrypt(k.master, ciphertext, plaintext)
}

// EncryptTo encrypts and signs data with the master key. The returned
// io.Writer writes IV || Ciphertext || HMAC. For the hash function, SHA256 is
// used.
func (k *Key) EncryptTo(wr io.Writer) io.WriteCloser {
	return crypto.EncryptTo(k.master, wr)
}

// Decrypt verifes and decrypts the ciphertext with the master key. Ciphertext
// must be in the form IV || Ciphertext || MAC.
func (k *Key) Decrypt(plaintext, ciphertext []byte) ([]byte, error) {
	return crypto.Decrypt(k.master, plaintext, ciphertext)
}

// DecryptFrom verifies and decrypts the ciphertext read from rd and makes it
// available on the returned Reader. Ciphertext must be in the form IV ||
// Ciphertext || MAC. In order to correctly verify the ciphertext, rd is
// drained, locally buffered and made available on the returned Reader
// afterwards. If an MAC verification failure is observed, it is returned
// immediately.
func (k *Key) DecryptFrom(rd io.Reader) (io.ReadCloser, error) {
	return crypto.DecryptFrom(k.master, rd)
}

// Master() returns the master keys for this repository. Only included for
// debug purposes.
func (k *Key) Master() *crypto.Key {
	return k.master
}

// User() returns the user keys for this key. Only included for debug purposes.
func (k *Key) User() *crypto.Key {
	return k.user
}

func (k *Key) String() string {
	if k == nil {
		return "<Key nil>"
	}
	return fmt.Sprintf("<Key of %s@%s, created on %s>", k.Username, k.Hostname, k.Created)
}

func (k Key) Name() string {
	return k.name
}
