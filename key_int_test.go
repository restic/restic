package khepri

import (
	"bytes"
	"encoding/hex"
	"testing"
)

var test_values = []struct {
	ekey, skey   []byte
	ciphertext   []byte
	plaintext    []byte
	should_panic bool
}{
	{
		ekey:       decode_hex("303e8687b1d7db18421bdc6bb8588ccadac4d59ee87b8ff70c44e635790cafef"),
		skey:       decode_hex("cc8d4b948ee0ebfe1d415de921d10353ef4d8824cb80b2bcc5fbff8a9b12a42c"),
		ciphertext: decode_hex("fe85b32b108308f6f8834a96e463b66e0eae6a0f1e9809da0773a2db12a24528bce3220e6a5700b40bd45ef2a2ce96a7fc0a895a019d4a77eef5fc9579297059c6d0"),
		plaintext:  []byte("Dies ist ein Test!"),
	},
}

func decode_hex(s string) []byte {
	d, _ := hex.DecodeString(s)
	return d
}

// returns true if function called panic
func should_panic(f func()) (did_panic bool) {
	defer func() {
		if r := recover(); r != nil {
			did_panic = true
		}
	}()

	f()

	return false
}

func TestCrypto(t *testing.T) {
	r := &Key{}

	for _, tv := range test_values {
		// test encryption
		r.master = &keys{
			Encrypt: tv.ekey,
			Sign:    tv.skey,
		}

		msg, err := r.encrypt(r.master, tv.plaintext)
		if err != nil {
			t.Fatal(err)
		}

		// decrypt message
		_, err = r.decrypt(r.master, msg)
		if err != nil {
			t.Fatal(err)
		}

		// change hmac, this must fail
		msg[len(msg)-8] ^= 0x23

		if _, err = r.decrypt(r.master, msg); err != ErrUnauthenticated {
			t.Fatal("wrong HMAC value not detected")
		}

		// test decryption
		p, err := r.decrypt(r.master, tv.ciphertext)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(p, tv.plaintext) {
			t.Fatalf("wrong plaintext: expected %q but got %q\n", tv.plaintext, p)
		}
	}
}
