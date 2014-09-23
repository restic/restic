package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"

	"code.google.com/p/go.crypto/scrypt"
	"code.google.com/p/go.crypto/ssh/terminal"

	"github.com/jessevdk/go-flags"
)

const (
	scrypt_N   = 65536
	scrypt_r   = 8
	scrypt_p   = 1
	aesKeySize = 32 // for AES256
)

var Opts struct {
	Password string `short:"p" long:"password"    description:"Password for the file"`
	Keys     string `short:"k" long:"keys"        description:"Keys for the file (encryption_key || sign_key, hex-encoded)"`
	Salt     string `short:"s" long:"salt"        description:"Salt to use (hex-encoded)"`
}

func newIV() ([]byte, error) {
	buf := make([]byte, aes.BlockSize)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func pad(plaintext []byte) []byte {
	l := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	if l == 0 {
		l = aes.BlockSize
	}

	if l <= 0 || l > aes.BlockSize {
		panic("invalid padding size")
	}

	return append(plaintext, bytes.Repeat([]byte{byte(l)}, l)...)
}

func unpad(plaintext []byte) []byte {
	l := len(plaintext)
	pad := plaintext[l-1]

	if pad > aes.BlockSize {
		panic(errors.New("padding > BlockSize"))
	}

	if pad == 0 {
		panic(errors.New("invalid padding 0"))
	}

	for i := l - int(pad); i < l; i++ {
		if plaintext[i] != pad {
			panic(errors.New("invalid padding!"))
		}
	}

	return plaintext[:l-int(pad)]
}

// Encrypt encrypts and signs data. Returned is IV || Ciphertext || HMAC. For
// the hash function, SHA256 is used, so the overhead is 16+32=48 byte.
func Encrypt(ekey, skey []byte, plaintext []byte) ([]byte, error) {
	iv, err := newIV()
	if err != nil {
		panic(fmt.Sprintf("unable to generate new random iv: %v", err))
	}

	c, err := aes.NewCipher(ekey)
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	e := cipher.NewCBCEncrypter(c, iv)
	p := pad(plaintext)
	ciphertext := make([]byte, len(p))
	e.CryptBlocks(ciphertext, p)

	ciphertext = append(iv, ciphertext...)

	hm := hmac.New(sha256.New, skey)

	n, err := hm.Write(ciphertext)
	if err != nil || n != len(ciphertext) {
		panic(fmt.Sprintf("unable to calculate hmac of ciphertext: %v", err))
	}

	return hm.Sum(ciphertext), nil
}

// Decrypt verifes and decrypts the ciphertext. Ciphertext must be in the form
// IV || Ciphertext || HMAC.
func Decrypt(ekey, skey []byte, ciphertext []byte) ([]byte, error) {
	hm := hmac.New(sha256.New, skey)

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
		panic("HMAC verification failed")
	}

	// extract iv
	iv, ciphertext := ciphertext[:aes.BlockSize], ciphertext[aes.BlockSize:]

	// decrypt data
	c, err := aes.NewCipher(ekey)
	if err != nil {
		panic(fmt.Sprintf("unable to create cipher: %v", err))
	}

	// decrypt
	e := cipher.NewCBCDecrypter(c, iv)
	plaintext := make([]byte, len(ciphertext))
	e.CryptBlocks(plaintext, ciphertext)

	// remove padding and return
	return unpad(plaintext), nil
}

func errx(code int, format string, data ...interface{}) {
	if len(format) > 0 && format[len(format)-1] != '\n' {
		format += "\n"
	}
	fmt.Fprintf(os.Stderr, format, data...)
	os.Exit(code)
}

func read_password(prompt string) string {
	p := os.Getenv("KHEPRI_PASSWORD")
	if p != "" {
		return p
	}

	fmt.Print(prompt)
	pw, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		errx(2, "unable to read password: %v", err)
	}
	fmt.Println()

	return string(pw)
}

func main() {
	args, err := flags.Parse(&Opts)
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		os.Exit(0)
	}

	var keys []byte

	if Opts.Password == "" && Opts.Keys == "" {
		Opts.Password = read_password("password: ")

		salt, err := hex.DecodeString(Opts.Salt)
		if err != nil {
			errx(1, "unable to hex-decode salt: %v", err)
		}

		keys, err = scrypt.Key([]byte(Opts.Password), salt, scrypt_N, scrypt_r, scrypt_p, 2*aesKeySize)
		if err != nil {
			errx(1, "scrypt: %v", err)
		}
	}

	if Opts.Keys != "" {
		keys, err = hex.DecodeString(Opts.Keys)
		if err != nil {
			errx(1, "unable to hex-decode keys: %v", err)
		}
	}

	if len(keys) != 2*aesKeySize {
		errx(2, "key length is not 512")
	}

	encrypt_key := keys[:aesKeySize]
	sign_key := keys[aesKeySize:]

	for _, filename := range args {
		f, err := os.Open(filename)
		defer f.Close()
		if err != nil {
			errx(3, "%v\n", err)
		}

		buf, err := ioutil.ReadAll(f)
		if err != nil {
			errx(3, "%v\n", err)
		}

		buf, err = Decrypt(encrypt_key, sign_key, buf)
		if err != nil {
			errx(3, "%v\n", err)
		}

		_, err = os.Stdout.Write(buf)
		if err != nil {
			errx(3, "%v\n", err)
		}
	}

}
