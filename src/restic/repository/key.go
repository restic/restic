package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"restic"
	"time"

	"restic/errors"

	"restic/backend"
	"restic/crypto"
	"restic/debug"
)

var (
	// ErrNoKeyFound is returned when no key for the repository could be decrypted.
	ErrNoKeyFound = errors.New("wrong password or no key found")

	// ErrMaxKeysReached is returned when the maximum number of keys was checked and no key could be found.
	ErrMaxKeysReached = errors.New("maximum number of keys reached")
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

// KDFParams tracks the parameters used for the KDF. If not set, it will be
// calibrated on the first run of AddKey().
var KDFParams *crypto.KDFParams

var (
	// KDFTimeout specifies the maximum runtime for the KDF.
	KDFTimeout = 500 * time.Millisecond

	// KDFMemory limits the memory the KDF is allowed to use.
	KDFMemory = 60
)

// createMasterKey creates a new master key in the given backend and encrypts
// it with the password.
func createMasterKey(s *Repository, password string) (*Key, error) {
	return AddKey(s, password, nil)
}

// OpenKey tries do decrypt the key specified by name with the given password.
func OpenKey(s *Repository, name string, password string) (*Key, error) {
	k, err := LoadKey(s, name)
	if err != nil {
		debug.Log("LoadKey(%v) returned error %v", name[:12], err)
		return nil, err
	}

	// check KDF
	if k.KDF != "scrypt" {
		return nil, errors.New("only supported KDF is scrypt()")
	}

	// derive user key
	params := crypto.KDFParams{
		N: k.N,
		R: k.R,
		P: k.P,
	}
	k.user, err = crypto.KDF(params, k.Salt, password)
	if err != nil {
		return nil, errors.Wrap(err, "crypto.KDF")
	}

	// decrypt master keys
	buf := make([]byte, len(k.Data))
	n, err := crypto.Decrypt(k.user, buf, k.Data)
	if err != nil {
		return nil, err
	}
	buf = buf[:n]

	// restore json
	k.master = &crypto.Key{}
	err = json.Unmarshal(buf, k.master)
	if err != nil {
		debug.Log("Unmarshal() returned error %v", err)
		return nil, errors.Wrap(err, "Unmarshal")
	}
	k.name = name

	if !k.Valid() {
		return nil, errors.New("Invalid key for repository")
	}

	return k, nil
}

// SearchKey tries to decrypt at most maxKeys keys in the backend with the
// given password. If none could be found, ErrNoKeyFound is returned. When
// maxKeys is reached, ErrMaxKeysReached is returned. When setting maxKeys to
// zero, all keys in the repo are checked.
func SearchKey(s *Repository, password string, maxKeys int) (*Key, error) {
	checked := 0

	// try at most maxKeysForSearch keys in repo
	done := make(chan struct{})
	defer close(done)
	for name := range s.Backend().List(restic.KeyFile, done) {
		if maxKeys > 0 && checked > maxKeys {
			return nil, ErrMaxKeysReached
		}

		debug.Log("trying key %v", name[:12])
		key, err := OpenKey(s, name, password)
		if err != nil {
			debug.Log("key %v returned error %v", name[:12], err)

			// ErrUnauthenticated means the password is wrong, try the next key
			if errors.Cause(err) == crypto.ErrUnauthenticated {
				continue
			}

			return nil, err
		}

		debug.Log("successfully opened key %v", name[:12])
		return key, nil
	}

	return nil, ErrNoKeyFound
}

// LoadKey loads a key from the backend.
func LoadKey(s *Repository, name string) (k *Key, err error) {
	h := restic.Handle{Type: restic.KeyFile, Name: name}
	data, err := backend.LoadAll(s.be, h, nil)
	if err != nil {
		return nil, err
	}

	k = &Key{}
	err = json.Unmarshal(data, k)
	if err != nil {
		return nil, errors.Wrap(err, "Unmarshal")
	}

	return k, nil
}

// AddKey adds a new key to an already existing repository.
func AddKey(s *Repository, password string, template *crypto.Key) (*Key, error) {
	// make sure we have valid KDF parameters
	if KDFParams == nil {
		p, err := crypto.Calibrate(KDFTimeout, KDFMemory)
		if err != nil {
			return nil, errors.Wrap(err, "Calibrate")
		}

		KDFParams = &p
		debug.Log("calibrated KDF parameters are %v", p)
	}

	// fill meta data about key
	newkey := &Key{
		Created: time.Now(),
		KDF:     "scrypt",
		N:       KDFParams.N,
		R:       KDFParams.R,
		P:       KDFParams.P,
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
	newkey.Salt, err = crypto.NewSalt()
	if err != nil {
		panic("unable to read enough random bytes for salt: " + err.Error())
	}

	// call KDF to derive user key
	newkey.user, err = crypto.KDF(*KDFParams, newkey.Salt, password)
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
		return nil, errors.Wrap(err, "Marshal")
	}

	newkey.Data, err = crypto.Encrypt(newkey.user, nil, buf)

	// dump as json
	buf, err = json.Marshal(newkey)
	if err != nil {
		return nil, errors.Wrap(err, "Marshal")
	}

	// store in repository and return
	h := restic.Handle{
		Type: restic.KeyFile,
		Name: restic.Hash(buf).String(),
	}

	err = s.be.Save(h, buf)
	if err != nil {
		return nil, err
	}

	newkey.name = h.Name

	return newkey, nil
}

func (k *Key) String() string {
	if k == nil {
		return "<Key nil>"
	}
	return fmt.Sprintf("<Key of %s@%s, created on %s>", k.Username, k.Hostname, k.Created)
}

// Name returns an identifier for the key.
func (k Key) Name() string {
	return k.name
}

// Valid tests whether the mac and encryption keys are valid (i.e. not zero)
func (k *Key) Valid() bool {
	return k.user.Valid() && k.master.Valid()
}
