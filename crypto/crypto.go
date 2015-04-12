package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/restic/restic/chunker"
	"golang.org/x/crypto/poly1305"
	"golang.org/x/crypto/scrypt"
)

const (
	aesKeySize  = 32                        // for AES256
	macKeySizeK = 16                        // for AES-128
	macKeySizeR = 16                        // for Poly1305
	macKeySize  = macKeySizeK + macKeySizeR // for Poly1305-AES128
	ivSize      = aes.BlockSize

	macSize   = poly1305.TagSize // Poly1305 size is 16 byte
	Extension = ivSize + macSize
)

var (
	// ErrUnauthenticated is returned when ciphertext verification has failed.
	ErrUnauthenticated = errors.New("ciphertext verification failed")

	// ErrBufferTooSmall is returned when the destination slice is too small
	// for the ciphertext.
	ErrBufferTooSmall = errors.New("destination buffer too small")
)

// Key holds signing and encryption keys for a repository. It is stored
// encrypted and signed as a JSON data structure in the Data field of the Key
// structure. For the master key, the secret random polynomial used for content
// defined chunking is included.
type Key struct {
	Sign              SigningKey    `json:"sign"`
	Encrypt           EncryptionKey `json:"encrypt"`
	ChunkerPolynomial chunker.Pol   `json:"chunker_polynomial,omitempty"`
}

type EncryptionKey [32]byte
type SigningKey struct {
	K [16]byte `json:"k"` // for AES128
	R [16]byte `json:"r"` // for Poly1305
}
type iv [ivSize]byte

// mask for key, (cf. http://cr.yp.to/mac/poly1305-20050329.pdf)
var poly1305KeyMask = [16]byte{
	0xff,
	0xff,
	0xff,
	0x0f, // 3: top four bits zero
	0xfc, // 4: bottom two bits zero
	0xff,
	0xff,
	0x0f, // 7: top four bits zero
	0xfc, // 8: bottom two bits zero
	0xff,
	0xff,
	0x0f, // 11: top four bits zero
	0xfc, // 12: bottom two bits zero
	0xff,
	0xff,
	0x0f, // 15: top four bits zero
}

// key is a [32]byte, in the form k||r
func poly1305_sign(msg []byte, nonce []byte, key *SigningKey) []byte {
	// prepare key for low-level poly1305.Sum(): r||n
	var k [32]byte

	// make sure key is masked
	maskKey(key)

	// fill in nonce, encrypted with AES and key[:16]
	cipher, err := aes.NewCipher(key.K[:])
	if err != nil {
		panic(err)
	}
	cipher.Encrypt(k[16:], nonce[:])

	// copy r
	copy(k[:16], key.R[:])

	// save mac in out
	var out [16]byte
	poly1305.Sum(&out, msg, &k)

	return out[:]
}

// mask poly1305 key
func maskKey(k *SigningKey) {
	if k == nil {
		return
	}
	for i := 0; i < poly1305.TagSize; i++ {
		k.R[i] = k.R[i] & poly1305KeyMask[i]
	}
}

// construct mac key from slice (k||r), with masking
func macKeyFromSlice(mk *SigningKey, data []byte) {
	copy(mk.K[:], data[:16])
	copy(mk.R[:], data[16:32])
	maskKey(mk)
}

// key: k||r
func poly1305_verify(msg []byte, nonce []byte, key *SigningKey, mac []byte) bool {
	// prepare key for low-level poly1305.Sum(): r||n
	var k [32]byte

	// make sure key is masked
	maskKey(key)

	// fill in nonce, encrypted with AES and key[:16]
	cipher, err := aes.NewCipher(key.K[:])
	if err != nil {
		panic(err)
	}
	cipher.Encrypt(k[16:], nonce[:])

	// copy r
	copy(k[:16], key.R[:])

	// copy mac to array
	var m [16]byte
	copy(m[:], mac)

	return poly1305.Verify(&m, msg, &k)
}

// NewKey returns new encryption and signing keys.
func NewKey() (k *Key) {
	k = &Key{}
	n, err := rand.Read(k.Encrypt[:])
	if n != aesKeySize || err != nil {
		panic("unable to read enough random bytes for encryption key")
	}

	n, err = rand.Read(k.Sign.K[:])
	if n != macKeySizeK || err != nil {
		panic("unable to read enough random bytes for mac encryption key")
	}

	n, err = rand.Read(k.Sign.R[:])
	if n != macKeySizeR || err != nil {
		panic("unable to read enough random bytes for mac signing key")
	}
	// mask r
	maskKey(&k.Sign)

	return k
}

func newIV() (iv iv) {
	n, err := rand.Read(iv[:])
	if n != ivSize || err != nil {
		panic("unable to read enough random bytes for iv")
	}
	return
}

type jsonMACKey struct {
	K []byte `json:"k"`
	R []byte `json:"r"`
}

func (m *SigningKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonMACKey{K: m.K[:], R: m.R[:]})
}

func (m *SigningKey) UnmarshalJSON(data []byte) error {
	j := jsonMACKey{}
	err := json.Unmarshal(data, &j)
	if err != nil {
		return err
	}
	copy(m.K[:], j.K)
	copy(m.R[:], j.R)

	return nil
}

func (k *EncryptionKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(k[:])
}

func (k *EncryptionKey) UnmarshalJSON(data []byte) error {
	d := make([]byte, aesKeySize)
	err := json.Unmarshal(data, &d)
	if err != nil {
		return err
	}
	copy(k[:], d)

	return nil
}

// Encrypt encrypts and signs data. Stored in ciphertext is IV || Ciphertext ||
// MAC. Encrypt returns the ciphertext's length.
func Encrypt(ks *Key, ciphertext, plaintext []byte) (int, error) {
	if cap(ciphertext) < len(plaintext)+ivSize+macSize {
		return 0, ErrBufferTooSmall
	}

	iv := newIV()
	copy(ciphertext, iv[:])

	c, err := aes.NewCipher(ks.Encrypt[:])
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	e := cipher.NewCTR(c, ciphertext[:ivSize])

	e.XORKeyStream(ciphertext[ivSize:cap(ciphertext)], plaintext)
	ciphertext = ciphertext[:ivSize+len(plaintext)]

	mac := poly1305_sign(ciphertext[ivSize:], ciphertext[:ivSize], &ks.Sign)
	ciphertext = append(ciphertext, mac...)

	return len(ciphertext), nil
}

// Decrypt verifies and decrypts the ciphertext. Ciphertext must be in the form
// IV || Ciphertext || MAC.
func Decrypt(ks *Key, plaintext, ciphertext []byte) ([]byte, error) {
	// check for plausible length
	if len(ciphertext) < ivSize+macSize {
		panic("trying to decrypt invalid data: ciphertext too small")
	}

	if cap(plaintext) < len(ciphertext) {
		// extend plaintext
		plaintext = append(plaintext, make([]byte, len(ciphertext)-cap(plaintext))...)
	}

	// extract mac
	l := len(ciphertext) - macSize
	ciphertext, mac := ciphertext[:l], ciphertext[l:]

	// verify mac
	if !poly1305_verify(ciphertext[ivSize:], ciphertext[:ivSize], &ks.Sign, mac) {
		return nil, ErrUnauthenticated
	}

	// extract iv
	iv, ciphertext := ciphertext[:ivSize], ciphertext[ivSize:]

	// decrypt data
	c, err := aes.NewCipher(ks.Encrypt[:])
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	// decrypt
	e := cipher.NewCTR(c, iv)
	plaintext = plaintext[:len(ciphertext)]
	e.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}

// KDF derives encryption and signing keys from the password using the supplied
// parameters N, R and P and the Salt.
func KDF(N, R, P int, salt []byte, password string) (*Key, error) {
	if len(salt) == 0 {
		return nil, fmt.Errorf("scrypt() called with empty salt")
	}

	derKeys := &Key{}

	keybytes := macKeySize + aesKeySize
	scryptKeys, err := scrypt.Key([]byte(password), salt, N, R, P, keybytes)
	if err != nil {
		return nil, fmt.Errorf("error deriving keys from password: %v", err)
	}

	if len(scryptKeys) != keybytes {
		return nil, fmt.Errorf("invalid numbers of bytes expanded from scrypt(): %d", len(scryptKeys))
	}

	// first 32 byte of scrypt output is the encryption key
	copy(derKeys.Encrypt[:], scryptKeys[:aesKeySize])

	// next 32 byte of scrypt output is the mac key, in the form k||r
	macKeyFromSlice(&derKeys.Sign, scryptKeys[aesKeySize:])

	return derKeys, nil
}

type encryptWriter struct {
	iv      iv
	wroteIV bool
	data    *bytes.Buffer
	key     *Key
	s       cipher.Stream
	w       io.Writer
	origWr  io.Writer
	err     error // remember error writing iv
}

func (e *encryptWriter) Close() error {
	// write mac
	mac := poly1305_sign(e.data.Bytes()[ivSize:], e.data.Bytes()[:ivSize], &e.key.Sign)
	_, err := e.origWr.Write(mac)
	if err != nil {
		return err
	}

	// return buffer
	bufPool.Put(e.data.Bytes())

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
		_, e.err = e.origWr.Write(e.iv[:])
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

// EncryptTo buffers data written to the returned io.WriteCloser. When Close()
// is called, the data is encrypted an written to the underlying writer.
func EncryptTo(ks *Key, wr io.Writer) io.WriteCloser {
	ew := &encryptWriter{
		iv:     newIV(),
		data:   bytes.NewBuffer(getBuffer()[:0]),
		key:    ks,
		origWr: wr,
	}

	// buffer iv for mac
	_, err := ew.data.Write(ew.iv[:])
	if err != nil {
		panic(err)
	}

	c, err := aes.NewCipher(ks.Encrypt[:])
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	ew.s = cipher.NewCTR(c, ew.iv[:])
	ew.w = io.MultiWriter(ew.data, wr)

	return ew
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
		d.Close()
		return n, io.EOF
	}

	n := copy(dst, d.buf[d.pos:d.pos+len(dst)])
	d.pos += n

	return n, nil
}

func (d *decryptReader) ReadByte() (c byte, err error) {
	if d.buf == nil {
		return 0, io.EOF
	}

	remaining := len(d.buf) - d.pos
	if remaining == 1 {
		c = d.buf[d.pos]
		d.Close()
		return c, io.EOF
	}

	c = d.buf[d.pos]
	d.pos++

	return
}

func (d *decryptReader) Close() error {
	if d.buf == nil {
		return nil
	}

	freeBuffer(d.buf)
	d.buf = nil
	return nil
}

// DecryptFrom verifies and decrypts the ciphertext read from rd with ks and
// makes it available on the returned Reader. Ciphertext must be in the form IV
// || Ciphertext || MAC. In order to correctly verify the ciphertext, rd is
// drained, locally buffered and made available on the returned Reader
// afterwards. If a MAC verification failure is observed, it is returned
// immediately.
func DecryptFrom(ks *Key, rd io.Reader) (io.ReadCloser, error) {
	ciphertext := getBuffer()

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

	// decrypt
	ciphertext, err = Decrypt(ks, ciphertext, ciphertext)
	if err != nil {
		return nil, err
	}

	return &decryptReader{buf: ciphertext}, nil
}
