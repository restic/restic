package crypto

import (
	"fmt"

	"golang.org/x/crypto/scrypt"
)

// KDF derives encryption and message authentication keys from the password
// using the supplied parameters N, R and P and the Salt.
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
	macKeyFromSlice(&derKeys.MAC, scryptKeys[aesKeySize:])

	return derKeys, nil
}
