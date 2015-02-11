package restic

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/user"
	"time"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"

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

	id backend.ID
}

// keys is a JSON structure that holds signing and encryption keys.
type keys struct {
	Sign    []byte
	Encrypt []byte
}

// CreateKey initializes a master key in the given backend and encrypts it with
// the password.
func CreateKey(s Server, password string) (*Key, error) {
	return AddKey(s, password, nil)
}

// OpenKey tries do decrypt the key specified by id with the given password.
func OpenKey(s Server, id backend.ID, password string) (*Key, error) {
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

	// call scrypt() to derive user key
	newkey.user, err = newkey.scrypt(password)
	if err != nil {
		return nil, err
	}

	if template == nil {
		// generate new random master keys
		newkey.master, err = newkey.newKeys()
		if err != nil {
			return nil, err
		}
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
	id, err := s.Create(backend.Key, buf)
	if err != nil {
		return nil, err
	}
	newkey.id = id

	FreeChunkBuf("key", newkey.Data)

	return newkey, nil
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

type HashReader struct {
	r      io.Reader
	h      hash.Hash
	sum    []byte
	closed bool
}

func NewHashReader(r io.Reader, h hash.Hash) *HashReader {
	return &HashReader{
		h:   h,
		r:   io.TeeReader(r, h),
		sum: make([]byte, 0, h.Size()),
	}
}

func (h *HashReader) Read(p []byte) (n int, err error) {
	if !h.closed {
		n, err = h.r.Read(p)

		if err == io.EOF {
			h.closed = true
			h.sum = h.h.Sum(h.sum)
		} else if err != nil {
			return
		}
	}

	if h.closed {
		// output hash
		r := len(p) - n

		if r > 0 {
			c := copy(p[n:], h.sum)
			h.sum = h.sum[c:]

			n += c
			err = nil
		}

		if len(h.sum) == 0 {
			err = io.EOF
		}
	}

	return
}

// encryptFrom encrypts and signs data read from rd with ks. The returned
// io.Reader reads IV || Ciphertext || HMAC. For the hash function, SHA256 is
// used.
func (k *Key) encryptFrom(ks *keys, rd io.Reader) io.Reader {
	// create IV
	iv := make([]byte, ivSize)

	_, err := io.ReadFull(rand.Reader, iv)
	if err != nil {
		panic(fmt.Sprintf("unable to generate new random iv: %v", err))
	}

	c, err := aes.NewCipher(ks.Encrypt)
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	ivReader := bytes.NewReader(iv)

	encryptReader := cipher.StreamReader{
		R: rd,
		S: cipher.NewCTR(c, iv),
	}

	return NewHashReader(io.MultiReader(ivReader, encryptReader),
		hmac.New(sha256.New, ks.Sign))
}

// EncryptFrom encrypts and signs data read from rd with the master key. The
// returned io.Reader reads IV || Ciphertext || HMAC. For the hash function,
// SHA256 is used.
func (k *Key) EncryptFrom(rd io.Reader) io.Reader {
	return k.encryptFrom(k.master, rd)
}

// EncryptFrom encrypts and signs data read from rd with the user key. The
// returned io.Reader reads IV || Ciphertext || HMAC. For the hash function,
// SHA256 is used.
func (k *Key) EncryptUserFrom(rd io.Reader) io.Reader {
	return k.encryptFrom(k.user, rd)
}

// Decrypt verifes and decrypts the ciphertext. Ciphertext must be in the form
// IV || Ciphertext || HMAC.
func (k *Key) decrypt(ks *keys, ciphertext []byte) ([]byte, error) {
	// check for plausible length
	if len(ciphertext) < ivSize+hmacSize {
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

// DecryptUser verifes and decrypts the ciphertext with the user key. Ciphertext
// must be in the form IV || Ciphertext || HMAC.
func (k *Key) DecryptUser(ciphertext []byte) ([]byte, error) {
	return k.decrypt(k.user, ciphertext)
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
