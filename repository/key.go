package repository

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"time"

	"github.com/restic/restic/backend"
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

// createMasterKey creates a new master key in the given backend and encrypts
// it with the password.
func createMasterKey(s *Repository, password string) (*Key, error) {
	return AddKey(s, password, nil)
}

// OpenKey tries do decrypt the key specified by name with the given password.
func OpenKey(s *Repository, name string, password string) (*Key, error) {
	k, err := LoadKey(s, name)
	if err != nil {
		debug.Log("OpenKey", "LoadKey(%v) returned error %v", name[:12], err)
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
		debug.Log("OpenKey", "Unmarshal() returned error %v", err)
		return nil, err
	}
	k.name = name

	if !k.Valid() {
		return nil, errors.New("Invalid key for repository")
	}

	return k, nil
}

// SearchKey tries to decrypt all keys in the backend with the given password.
// If none could be found, ErrNoKeyFound is returned.
func SearchKey(s *Repository, password string) (*Key, error) {
	// try all keys in repo
	done := make(chan struct{})
	defer close(done)
	for name := range s.Backend().List(backend.Key, done) {
		debug.Log("SearchKey", "trying key %v", name[:12])
		key, err := OpenKey(s, name, password)
		if err != nil {
			debug.Log("SearchKey", "key %v returned error %v", name[:12], err)
			continue
		}

		debug.Log("SearchKey", "successfully opened key %v", name[:12])
		return key, nil
	}

	return nil, ErrNoKeyFound
}

// LoadKey loads a key from the backend.
func LoadKey(s *Repository, name string) (k *Key, err error) {
	// extract data from repo
	rd, err := s.be.Get(backend.Key, name)
	if err != nil {
		return nil, err
	}
	defer closeOrErr(rd, &err)

	// restore json
	dec := json.NewDecoder(rd)
	k = new(Key)
	err = dec.Decode(k)
	if err != nil {
		return nil, err
	}

	return k, nil
}

// AddKey adds a new key to an already existing repository.
func AddKey(s *Repository, password string, template *crypto.Key) (*Key, error) {
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
		newkey.master = crypto.NewRandomKey()
	} else {
		// copy master keys from old key
		newkey.master = template
	}

	// encrypt master keys (as json) with user key
	buf, err := json.Marshal(newkey.master)
	if err != nil {
		return nil, err
	}

	newkey.Data, err = crypto.Encrypt(newkey.user, nil, buf)

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

	name := hex.EncodeToString(plainhw.Sum(nil))

	err = blob.Finalize(backend.Key, name)
	if err != nil {
		return nil, err
	}

	newkey.name = name

	return newkey, nil
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

// Valid tests whether the mac and encryption keys are valid (i.e. not zero)
func (k *Key) Valid() bool {
	return k.user.Valid() && k.master.Valid()
}
