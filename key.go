package khepri

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"time"

	"github.com/fd0/khepri/backend"
	"github.com/fd0/khepri/chunker"

	"golang.org/x/crypto/scrypt"
)

// max size is 8MiB, defined in chunker
const ivSize = aes.BlockSize
const hmacSize = sha256.Size
const maxCiphertextSize = ivSize + chunker.MaxSize + hmacSize
const CiphertextExtension = ivSize + hmacSize

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
	aesKeysize     = 32 // for AES256
	hmacKeysize    = 32 // for HMAC with SHA256
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
}

// keys is a JSON structure that holds signing and encryption keys.
type keys struct {
	Sign    []byte
	Encrypt []byte
}

// CreateKey initializes a master key in the given backend and encrypts it with
// the password.
func CreateKey(be backend.Server, password string) (*Key, error) {
	// fill meta data about key
	k := &Key{
		Created: time.Now(),
		KDF:     "scrypt",
		N:       scryptN,
		R:       scryptR,
		P:       scryptP,
	}

	hn, err := os.Hostname()
	if err == nil {
		k.Hostname = hn
	}

	usr, err := user.Current()
	if err == nil {
		k.Username = usr.Username
	}

	// generate random salt
	k.Salt = make([]byte, scryptSaltsize)
	n, err := rand.Read(k.Salt)
	if n != scryptSaltsize || err != nil {
		panic("unable to read enough random bytes for salt")
	}

	// call scrypt() to derive user key
	k.user, err = k.scrypt(password)
	if err != nil {
		return nil, err
	}

	// generate new random master keys
	k.master, err = k.newKeys()
	if err != nil {
		return nil, err
	}

	// encrypt master keys (as json) with user key
	buf, err := json.Marshal(k.master)
	if err != nil {
		return nil, err
	}

	k.Data = GetChunkBuf("key")
	n, err = k.EncryptUser(k.Data, buf)
	k.Data = k.Data[:n]

	// dump as json
	buf, err = json.Marshal(k)
	if err != nil {
		return nil, err
	}

	// store in repository and return
	_, err = be.Create(backend.Key, buf)
	if err != nil {
		return nil, err
	}

	FreeChunkBuf("key", k.Data)

	return k, nil
}

// OpenKey tries do decrypt the key specified by id with the given password.
func OpenKey(be backend.Server, id backend.ID, password string) (*Key, error) {
	// extract data from repo
	data, err := be.Get(backend.Key, id)
	if err != nil {
		return nil, err
	}

	// restore json
	k := &Key{}
	err = json.Unmarshal(data, k)
	if err != nil {
		return nil, err
	}

	// check KDF
	if k.KDF != "scrypt" {
		return nil, errors.New("only supported KDF is scrypt()")
	}

	// derive user key
	k.user, err = k.scrypt(password)
	if err != nil {
		return nil, err
	}

	// decrypt master keys
	buf, err := k.DecryptUser(k.Data)
	if err != nil {
		return nil, err
	}

	// restore json
	k.master = &keys{}
	err = json.Unmarshal(buf, k.master)
	if err != nil {
		return nil, err
	}

	return k, nil
}

// SearchKey tries to decrypt all keys in the backend with the given password.
// If none could be found, ErrNoKeyFound is returned.
func SearchKey(be backend.Server, password string) (*Key, error) {
	// list all keys
	ids, err := be.List(backend.Key)
	if err != nil {
		panic(err)
	}

	// try all keys in repo
	var key *Key
	for _, id := range ids {
		key, err = OpenKey(be, id, password)
		if err != nil {
			continue
		}

		return key, nil
	}

	return nil, ErrNoKeyFound
}

func (k *Key) scrypt(password string) (*keys, error) {
	if len(k.Salt) == 0 {
		return nil, fmt.Errorf("scrypt() called with empty salt")
	}

	keybytes := hmacKeysize + aesKeysize
	scryptKeys, err := scrypt.Key([]byte(password), k.Salt, k.N, k.R, k.P, keybytes)
	if err != nil {
		return nil, fmt.Errorf("error deriving keys from password: %v", err)
	}

	if len(scryptKeys) != keybytes {
		return nil, fmt.Errorf("invalid numbers of bytes expanded from scrypt(): %d", len(scryptKeys))
	}

	ks := &keys{
		Encrypt: scryptKeys[:aesKeysize],
		Sign:    scryptKeys[aesKeysize:],
	}
	return ks, nil
}

func (k *Key) newKeys() (*keys, error) {
	ks := &keys{
		Encrypt: make([]byte, aesKeysize),
		Sign:    make([]byte, hmacKeysize),
	}
	n, err := rand.Read(ks.Encrypt)
	if n != aesKeysize || err != nil {
		panic("unable to read enough random bytes for encryption key")
	}
	n, err = rand.Read(ks.Sign)
	if n != hmacKeysize || err != nil {
		panic("unable to read enough random bytes for signing key")
	}

	return ks, nil
}

func (k *Key) newIV(buf []byte) error {
	_, err := io.ReadFull(rand.Reader, buf[:ivSize])
	buf = buf[:ivSize]
	if err != nil {
		return err
	}

	return nil
}

// Encrypt encrypts and signs data. Stored in ciphertext is IV || Ciphertext ||
// HMAC. Encrypt returns the ciphertext's length. For the hash function, SHA256
// is used, so the overhead is 16+32=48 byte.
func (k *Key) encrypt(ks *keys, ciphertext, plaintext []byte) (int, error) {
	if cap(ciphertext) < len(plaintext)+ivSize+hmacSize {
		return 0, ErrBufferTooSmall
	}

	_, err := io.ReadFull(rand.Reader, ciphertext[:ivSize])
	if err != nil {
		panic(fmt.Sprintf("unable to generate new random iv: %v", err))
	}

	c, err := aes.NewCipher(ks.Encrypt)
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	e := cipher.NewCTR(c, ciphertext[:ivSize])
	e.XORKeyStream(ciphertext[ivSize:cap(ciphertext)], plaintext)
	ciphertext = ciphertext[:ivSize+len(plaintext)]

	hm := hmac.New(sha256.New, ks.Sign)

	n, err := hm.Write(ciphertext)
	if err != nil || n != len(ciphertext) {
		panic(fmt.Sprintf("unable to calculate hmac of ciphertext: %v", err))
	}

	ciphertext = hm.Sum(ciphertext)

	return len(ciphertext), nil
}

// EncryptUser encrypts and signs data with the user key. Stored in ciphertext
// is IV || Ciphertext || HMAC. Returns the ciphertext length. For the hash
// function, SHA256 is used, so the overhead is 16+32=48 byte.
func (k *Key) EncryptUser(ciphertext, plaintext []byte) (int, error) {
	return k.encrypt(k.user, ciphertext, plaintext)
}

// Encrypt encrypts and signs data with the master key. Stored in ciphertext is
// IV || Ciphertext || HMAC. Returns the ciphertext length. For the hash
// function, SHA256 is used, so the overhead is 16+32=48 byte.
func (k *Key) Encrypt(ciphertext, plaintext []byte) (int, error) {
	return k.encrypt(k.master, ciphertext, plaintext)
}

// Decrypt verifes and decrypts the ciphertext. Ciphertext must be in the form
// IV || Ciphertext || HMAC.
func (k *Key) decrypt(ks *keys, ciphertext []byte) ([]byte, error) {
	// check for plausible length
	if len(ciphertext) <= ivSize+hmacSize {
		panic("trying to decryipt invalid data: ciphertext too small")
	}

	hm := hmac.New(sha256.New, ks.Sign)

	// extract hmac
	l := len(ciphertext) - hm.Size()
	ciphertext, mac := ciphertext[:l], ciphertext[l:]

	// calculate new hmac
	n, err := hm.Write(ciphertext)
	if err != nil || n != len(ciphertext) {
		panic(fmt.Sprintf("unable to calculate hmac of ciphertext, err %v", err))
	}

	// verify hmac
	mac2 := hm.Sum(nil)

	if !hmac.Equal(mac, mac2) {
		return nil, ErrUnauthenticated
	}

	// extract iv
	iv, ciphertext := ciphertext[:aes.BlockSize], ciphertext[aes.BlockSize:]

	// decrypt data
	c, err := aes.NewCipher(ks.Encrypt)
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	// decrypt
	e := cipher.NewCTR(c, iv)
	plaintext := make([]byte, len(ciphertext))
	e.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}

// Decrypt verifes and decrypts the ciphertext with the master key. Ciphertext
// must be in the form IV || Ciphertext || HMAC.
func (k *Key) Decrypt(ciphertext []byte) ([]byte, error) {
	return k.decrypt(k.master, ciphertext)
}

// DecryptUser verifes and decrypts the ciphertext with the master key. Ciphertext
// must be in the form IV || Ciphertext || HMAC.
func (k *Key) DecryptUser(ciphertext []byte) ([]byte, error) {
	return k.decrypt(k.user, ciphertext)
}

// Each calls backend.Each() with the given parameters, Decrypt() on the
// ciphertext and, on successful decryption, f with the plaintext.
func (k *Key) Each(be backend.Server, t backend.Type, f func(backend.ID, []byte, error)) error {
	return backend.Each(be, t, func(id backend.ID, data []byte, e error) {
		if e != nil {
			f(id, nil, e)
			return
		}

		buf, err := k.Decrypt(data)
		if err != nil {
			f(id, nil, err)
			return
		}

		f(id, buf, nil)
	})
}

func (k *Key) String() string {
	if k == nil {
		return "<Key nil>"
	}
	return fmt.Sprintf("<Key of %s@%s, created on %s>", k.Username, k.Hostname, k.Created)
}
