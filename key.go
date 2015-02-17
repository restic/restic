package restic

import (
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
	"io/ioutil"
	"os"
	"os/user"
	"sync"
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

type encryptWriter struct {
	iv      []byte
	wroteIV bool
	h       hash.Hash
	s       cipher.Stream
	w       io.Writer
	origWr  io.Writer
	err     error // remember error writing iv
}

func (e *encryptWriter) Close() error {
	// write hmac
	_, err := e.origWr.Write(e.h.Sum(nil))
	if err != nil {
		return err
	}

	return nil
}

const encryptWriterChunkSize = 512 * 1024 // 512 KiB
var encryptWriterBufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, encryptWriterChunkSize)
	},
}

func (e *encryptWriter) Write(p []byte) (int, error) {
	// write iv first
	if !e.wroteIV {
		_, e.err = e.origWr.Write(e.iv)
		e.wroteIV = true
	}

	if e.err != nil {
		return 0, e.err
	}

	buf := encryptWriterBufPool.Get().([]byte)
	defer encryptWriterBufPool.Put(buf)

	written := 0
	for len(p) > 0 {
		max := len(p)
		if max > encryptWriterChunkSize {
			max = encryptWriterChunkSize
		}

		e.s.XORKeyStream(buf, p[:max])
		n, err := e.w.Write(buf[:max])
		if n != max {
			if err == nil { // should never happen
				err = io.ErrShortWrite
			}
		}

		written += n
		p = p[n:]

		if err != nil {
			e.err = err
			return written, err
		}
	}

	return written, nil
}

func (k *Key) encryptTo(ks *keys, wr io.Writer) io.WriteCloser {
	ew := &encryptWriter{
		iv:     make([]byte, ivSize),
		h:      hmac.New(sha256.New, ks.Sign),
		origWr: wr,
	}

	_, err := io.ReadFull(rand.Reader, ew.iv)
	if err != nil {
		panic(fmt.Sprintf("unable to generate new random iv: %v", err))
	}

	// write iv to hmac
	_, err = ew.h.Write(ew.iv)
	if err != nil {
		panic(err)
	}

	c, err := aes.NewCipher(ks.Encrypt)
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	ew.s = cipher.NewCTR(c, ew.iv)
	ew.w = io.MultiWriter(ew.h, wr)

	return ew
}

// EncryptTo encrypts and signs data with the master key. The returned
// io.Writer writes IV || Ciphertext || HMAC. For the hash function, SHA256 is
// used.
func (k *Key) EncryptTo(wr io.Writer) io.WriteCloser {
	return k.encryptTo(k.master, wr)
}

// EncryptUserTo encrypts and signs data with the user key. The returned
// io.Writer writes IV || Ciphertext || HMAC. For the hash function, SHA256 is
// used.
func (k *Key) EncryptUserTo(wr io.Writer) io.WriteCloser {
	return k.encryptTo(k.user, wr)
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

type decryptReader struct {
	buf []byte
	pos int
}

func (d *decryptReader) Read(dst []byte) (int, error) {
	if d.buf == nil {
		return 0, io.EOF
	}

	if len(dst) == 0 {
		return 0, nil
	}

	remaining := len(d.buf) - d.pos
	if len(dst) >= remaining {
		n := copy(dst, d.buf[d.pos:])
		FreeChunkBuf("decryptReader", d.buf)
		d.buf = nil
		return n, io.EOF
	}

	n := copy(dst, d.buf[d.pos:d.pos+len(dst)])
	d.pos += n

	return n, nil
}

func (d *decryptReader) Close() error {
	if d.buf == nil {
		return nil
	}

	FreeChunkBuf("decryptReader", d.buf)
	d.buf = nil
	return nil
}

// decryptFrom verifies and decrypts the ciphertext read from rd with ks and
// makes it available on the returned Reader. Ciphertext must be in the form IV
// || Ciphertext || HMAC. In order to correctly verify the ciphertext, rd is
// drained, locally buffered and made available on the returned Reader
// afterwards. If an HMAC verification failure is observed, it is returned
// immediately.
func (k *Key) decryptFrom(ks *keys, rd io.Reader) (io.ReadCloser, error) {
	ciphertext := GetChunkBuf("decryptReader")
	ciphertext = ciphertext[0:cap(ciphertext)]
	n, err := io.ReadFull(rd, ciphertext)
	if err != io.ErrUnexpectedEOF {
		// read remaining data
		buf, e := ioutil.ReadAll(rd)
		ciphertext = append(ciphertext, buf...)
		n += len(buf)
		err = e
	} else {
		err = nil
	}

	if err != nil {
		return nil, err
	}

	ciphertext = ciphertext[:n]

	// check for plausible length
	if len(ciphertext) < ivSize+hmacSize {
		panic("trying to decrypt invalid data: ciphertext too small")
	}

	hm := hmac.New(sha256.New, ks.Sign)

	// extract hmac
	l := len(ciphertext) - hm.Size()
	ciphertext, mac := ciphertext[:l], ciphertext[l:]

	// calculate new hmac
	n, err = hm.Write(ciphertext)
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

	stream := cipher.NewCTR(c, iv)
	stream.XORKeyStream(ciphertext, ciphertext)

	return &decryptReader{buf: ciphertext}, nil
}

// DecryptFrom verifies and decrypts the ciphertext read from rd and makes it
// available on the returned Reader. Ciphertext must be in the form IV ||
// Ciphertext || HMAC. In order to correctly verify the ciphertext, rd is
// drained, locally buffered and made available on the returned Reader
// afterwards. If an HMAC verification failure is observed, it is returned
// immediately.
func (k *Key) DecryptFrom(rd io.Reader) (io.ReadCloser, error) {
	return k.decryptFrom(k.master, rd)
}

// DecryptFrom verifies and decrypts the ciphertext read from rd with the user
// key and makes it available on the returned Reader. Ciphertext must be in the
// form IV || Ciphertext || HMAC. In order to correctly verify the ciphertext,
// rd is drained, locally buffered and made available on the returned Reader
// afterwards. If an HMAC verification failure is observed, it is returned
// immediately.
func (k *Key) DecryptUserFrom(rd io.Reader) (io.ReadCloser, error) {
	return k.decryptFrom(k.user, rd)
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
