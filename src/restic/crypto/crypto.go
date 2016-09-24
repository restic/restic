package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"restic/errors"

	"golang.org/x/crypto/poly1305"
)

const (
	aesKeySize  = 32                        // for AES-256
	macKeySizeK = 16                        // for AES-128
	macKeySizeR = 16                        // for Poly1305
	macKeySize  = macKeySizeK + macKeySizeR // for Poly1305-AES128
	ivSize      = aes.BlockSize

	macSize   = poly1305.TagSize
	Extension = ivSize + macSize
)

var (
	// ErrUnauthenticated is returned when ciphertext verification has failed.
	ErrUnauthenticated = errors.New("ciphertext verification failed")
)

// Key holds encryption and message authentication keys for a repository. It is stored
// encrypted and authenticated as a JSON data structure in the Data field of the Key
// structure.
type Key struct {
	MAC     MACKey        `json:"mac"`
	Encrypt EncryptionKey `json:"encrypt"`
}

type EncryptionKey [32]byte
type MACKey struct {
	K [16]byte // for AES-128
	R [16]byte // for Poly1305

	masked bool // remember if the MAC key has already been masked
}

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

func poly1305MAC(msg []byte, nonce []byte, key *MACKey) []byte {
	k := poly1305PrepareKey(nonce, key)

	var out [16]byte
	poly1305.Sum(&out, msg, &k)

	return out[:]
}

// mask poly1305 key
func maskKey(k *MACKey) {
	if k == nil || k.masked {
		return
	}

	for i := 0; i < poly1305.TagSize; i++ {
		k.R[i] = k.R[i] & poly1305KeyMask[i]
	}

	k.masked = true
}

// construct mac key from slice (k||r), with masking
func macKeyFromSlice(mk *MACKey, data []byte) {
	copy(mk.K[:], data[:16])
	copy(mk.R[:], data[16:32])
	maskKey(mk)
}

// prepare key for low-level poly1305.Sum(): r||n
func poly1305PrepareKey(nonce []byte, key *MACKey) [32]byte {
	var k [32]byte

	maskKey(key)

	cipher, err := aes.NewCipher(key.K[:])
	if err != nil {
		panic(err)
	}
	cipher.Encrypt(k[16:], nonce[:])

	copy(k[:16], key.R[:])

	return k
}

func poly1305Verify(msg []byte, nonce []byte, key *MACKey, mac []byte) bool {
	k := poly1305PrepareKey(nonce, key)

	var m [16]byte
	copy(m[:], mac)

	return poly1305.Verify(&m, msg, &k)
}

// NewRandomKey returns new encryption and message authentication keys.
func NewRandomKey() *Key {
	k := &Key{}

	n, err := rand.Read(k.Encrypt[:])
	if n != aesKeySize || err != nil {
		panic("unable to read enough random bytes for encryption key")
	}

	n, err = rand.Read(k.MAC.K[:])
	if n != macKeySizeK || err != nil {
		panic("unable to read enough random bytes for MAC encryption key")
	}

	n, err = rand.Read(k.MAC.R[:])
	if n != macKeySizeR || err != nil {
		panic("unable to read enough random bytes for MAC key")
	}

	maskKey(&k.MAC)
	return k
}

func newIV() []byte {
	iv := make([]byte, ivSize)
	n, err := rand.Read(iv)
	if n != ivSize || err != nil {
		panic("unable to read enough random bytes for iv")
	}
	return iv
}

type jsonMACKey struct {
	K []byte `json:"k"`
	R []byte `json:"r"`
}

func (m *MACKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonMACKey{K: m.K[:], R: m.R[:]})
}

func (m *MACKey) UnmarshalJSON(data []byte) error {
	j := jsonMACKey{}
	err := json.Unmarshal(data, &j)
	if err != nil {
		return errors.Wrap(err, "Unmarshal")
	}
	copy(m.K[:], j.K)
	copy(m.R[:], j.R)

	return nil
}

// Valid tests whether the key k is valid (i.e. not zero).
func (k *MACKey) Valid() bool {
	nonzeroK := false
	for i := 0; i < len(k.K); i++ {
		if k.K[i] != 0 {
			nonzeroK = true
		}
	}

	if !nonzeroK {
		return false
	}

	for i := 0; i < len(k.R); i++ {
		if k.R[i] != 0 {
			return true
		}
	}

	return false
}

func (k *EncryptionKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(k[:])
}

func (k *EncryptionKey) UnmarshalJSON(data []byte) error {
	d := make([]byte, aesKeySize)
	err := json.Unmarshal(data, &d)
	if err != nil {
		return errors.Wrap(err, "Unmarshal")
	}
	copy(k[:], d)

	return nil
}

// Valid tests whether the key k is valid (i.e. not zero).
func (k *EncryptionKey) Valid() bool {
	for i := 0; i < len(k); i++ {
		if k[i] != 0 {
			return true
		}
	}

	return false
}

// ErrInvalidCiphertext is returned when trying to encrypt into the slice that
// holds the plaintext.
var ErrInvalidCiphertext = errors.New("invalid ciphertext, same slice used for plaintext")

// Encrypt encrypts and authenticates data. Stored in ciphertext is IV || Ciphertext ||
// MAC. Encrypt returns the new ciphertext slice, which is extended when
// necessary. ciphertext and plaintext may not point to (exactly) the same
// slice or non-intersecting slices.
func Encrypt(ks *Key, ciphertext []byte, plaintext []byte) ([]byte, error) {
	if !ks.Valid() {
		return nil, errors.New("invalid key")
	}

	ciphertext = ciphertext[:cap(ciphertext)]

	// test for same slice, if possible
	if len(plaintext) > 0 && len(ciphertext) > 0 && &plaintext[0] == &ciphertext[0] {
		return nil, ErrInvalidCiphertext
	}

	// extend ciphertext slice if necessary
	if len(ciphertext) < len(plaintext)+Extension {
		ext := len(plaintext) + Extension - cap(ciphertext)
		ciphertext = append(ciphertext, make([]byte, ext)...)
		ciphertext = ciphertext[:cap(ciphertext)]
	}

	iv := newIV()
	c, err := aes.NewCipher(ks.Encrypt[:])
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	e := cipher.NewCTR(c, iv[:])

	e.XORKeyStream(ciphertext[ivSize:], plaintext)
	copy(ciphertext, iv[:])

	// truncate to only cover iv and actual ciphertext
	ciphertext = ciphertext[:ivSize+len(plaintext)]

	mac := poly1305MAC(ciphertext[ivSize:], ciphertext[:ivSize], &ks.MAC)
	ciphertext = append(ciphertext, mac...)

	return ciphertext, nil
}

// Decrypt verifies and decrypts the ciphertext. Ciphertext must be in the form
// IV || Ciphertext || MAC. plaintext and ciphertext may point to (exactly) the
// same slice.
func Decrypt(ks *Key, plaintext []byte, ciphertextWithMac []byte) (int, error) {
	if !ks.Valid() {
		return 0, errors.New("invalid key")
	}

	// check for plausible length
	if len(ciphertextWithMac) < ivSize+macSize {
		panic("trying to decrypt invalid data: ciphertext too small")
	}

	// check buffer length for plaintext
	plaintextLength := len(ciphertextWithMac) - ivSize - macSize
	if len(plaintext) < plaintextLength {
		return 0, errors.Errorf("plaintext buffer too small, %d < %d", len(plaintext), plaintextLength)
	}

	// extract mac
	l := len(ciphertextWithMac) - macSize
	ciphertextWithIV, mac := ciphertextWithMac[:l], ciphertextWithMac[l:]

	// verify mac
	if !poly1305Verify(ciphertextWithIV[ivSize:], ciphertextWithIV[:ivSize], &ks.MAC, mac) {
		return 0, ErrUnauthenticated
	}

	// extract iv
	iv, ciphertext := ciphertextWithIV[:ivSize], ciphertextWithIV[ivSize:]

	if len(ciphertext) != plaintextLength {
		return 0, errors.Errorf("plaintext and ciphertext lengths do not match: %d != %d", len(ciphertext), plaintextLength)
	}

	// decrypt data
	c, err := aes.NewCipher(ks.Encrypt[:])
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	// decrypt
	e := cipher.NewCTR(c, iv)
	plaintext = plaintext[:len(ciphertext)]
	e.XORKeyStream(plaintext, ciphertext)

	return plaintextLength, nil
}

// Valid tests if the key is valid.
func (k *Key) Valid() bool {
	return k.Encrypt.Valid() && k.MAC.Valid()
}
