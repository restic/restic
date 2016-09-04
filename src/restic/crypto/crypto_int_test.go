package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// test vectors from http://cr.yp.to/mac/poly1305-20050329.pdf
var poly1305_tests = []struct {
	msg   []byte
	r     []byte
	k     []byte
	nonce []byte
	mac   []byte
}{
	{
		[]byte("\xf3\xf6"),
		[]byte("\x85\x1f\xc4\x0c\x34\x67\xac\x0b\xe0\x5c\xc2\x04\x04\xf3\xf7\x00"),
		[]byte("\xec\x07\x4c\x83\x55\x80\x74\x17\x01\x42\x5b\x62\x32\x35\xad\xd6"),
		[]byte("\xfb\x44\x73\x50\xc4\xe8\x68\xc5\x2a\xc3\x27\x5c\xf9\xd4\x32\x7e"),
		[]byte("\xf4\xc6\x33\xc3\x04\x4f\xc1\x45\xf8\x4f\x33\x5c\xb8\x19\x53\xde"),
	},
	{
		[]byte(""),
		[]byte("\xa0\xf3\x08\x00\x00\xf4\x64\x00\xd0\xc7\xe9\x07\x6c\x83\x44\x03"),
		[]byte("\x75\xde\xaa\x25\xc0\x9f\x20\x8e\x1d\xc4\xce\x6b\x5c\xad\x3f\xbf"),
		[]byte("\x61\xee\x09\x21\x8d\x29\xb0\xaa\xed\x7e\x15\x4a\x2c\x55\x09\xcc"),
		[]byte("\xdd\x3f\xab\x22\x51\xf1\x1a\xc7\x59\xf0\x88\x71\x29\xcc\x2e\xe7"),
	},
	{
		[]byte("\x66\x3c\xea\x19\x0f\xfb\x83\xd8\x95\x93\xf3\xf4\x76\xb6\xbc\x24\xd7\xe6\x79\x10\x7e\xa2\x6a\xdb\x8c\xaf\x66\x52\xd0\x65\x61\x36"),
		[]byte("\x48\x44\x3d\x0b\xb0\xd2\x11\x09\xc8\x9a\x10\x0b\x5c\xe2\xc2\x08"),
		[]byte("\x6a\xcb\x5f\x61\xa7\x17\x6d\xd3\x20\xc5\xc1\xeb\x2e\xdc\xdc\x74"),
		[]byte("\xae\x21\x2a\x55\x39\x97\x29\x59\x5d\xea\x45\x8b\xc6\x21\xff\x0e"),
		[]byte("\x0e\xe1\xc1\x6b\xb7\x3f\x0f\x4f\xd1\x98\x81\x75\x3c\x01\xcd\xbe"),
	}, {
		[]byte("\xab\x08\x12\x72\x4a\x7f\x1e\x34\x27\x42\xcb\xed\x37\x4d\x94\xd1\x36\xc6\xb8\x79\x5d\x45\xb3\x81\x98\x30\xf2\xc0\x44\x91\xfa\xf0\x99\x0c\x62\xe4\x8b\x80\x18\xb2\xc3\xe4\xa0\xfa\x31\x34\xcb\x67\xfa\x83\xe1\x58\xc9\x94\xd9\x61\xc4\xcb\x21\x09\x5c\x1b\xf9"),
		[]byte("\x12\x97\x6a\x08\xc4\x42\x6d\x0c\xe8\xa8\x24\x07\xc4\xf4\x82\x07"),
		[]byte("\xe1\xa5\x66\x8a\x4d\x5b\x66\xa5\xf6\x8c\xc5\x42\x4e\xd5\x98\x2d"),
		[]byte("\x9a\xe8\x31\xe7\x43\x97\x8d\x3a\x23\x52\x7c\x71\x28\x14\x9e\x3a"),
		[]byte("\x51\x54\xad\x0d\x2c\xb2\x6e\x01\x27\x4f\xc5\x11\x48\x49\x1f\x1b"),
	},
}

func TestPoly1305(t *testing.T) {
	for _, test := range poly1305_tests {
		key := &MACKey{}
		copy(key.K[:], test.k)
		copy(key.R[:], test.r)
		mac := poly1305MAC(test.msg, test.nonce, key)

		if !bytes.Equal(mac, test.mac) {
			t.Fatalf("wrong mac calculated, want: %02x, got: %02x", test.mac, mac)
		}

		if !poly1305Verify(test.msg, test.nonce, key, test.mac) {
			t.Fatalf("mac does not verify: mac: %02x", test.mac)
		}
	}
}

var testValues = []struct {
	ekey       EncryptionKey
	skey       MACKey
	ciphertext []byte
	plaintext  []byte
}{
	{
		ekey: EncryptionKey([...]byte{
			0x30, 0x3e, 0x86, 0x87, 0xb1, 0xd7, 0xdb, 0x18, 0x42, 0x1b, 0xdc, 0x6b, 0xb8, 0x58, 0x8c, 0xca,
			0xda, 0xc4, 0xd5, 0x9e, 0xe8, 0x7b, 0x8f, 0xf7, 0x0c, 0x44, 0xe6, 0x35, 0x79, 0x0c, 0xaf, 0xef,
		}),
		skey: MACKey{
			K: [...]byte{0xef, 0x4d, 0x88, 0x24, 0xcb, 0x80, 0xb2, 0xbc, 0xc5, 0xfb, 0xff, 0x8a, 0x9b, 0x12, 0xa4, 0x2c},
			R: [...]byte{0xcc, 0x8d, 0x4b, 0x94, 0x8e, 0xe0, 0xeb, 0xfe, 0x1d, 0x41, 0x5d, 0xe9, 0x21, 0xd1, 0x03, 0x53},
		},
		ciphertext: decodeHex("69fb41c62d12def4593bd71757138606338f621aeaeb39da0fe4f99233f8037a54ea63338a813bcf3f75d8c3cc75dddf8750"),
		plaintext:  []byte("Dies ist ein Test!"),
	},
}

func decodeHex(s string) []byte {
	d, _ := hex.DecodeString(s)
	return d
}

func TestCrypto(t *testing.T) {
	msg := make([]byte, 0, 8*1024*1024) // use 8MiB for now
	for _, tv := range testValues {
		// test encryption
		k := &Key{
			Encrypt: tv.ekey,
			MAC:     tv.skey,
		}

		msg, err := Encrypt(k, msg, tv.plaintext)
		if err != nil {
			t.Fatal(err)
		}

		// decrypt message
		buf := make([]byte, len(tv.plaintext))
		n, err := Decrypt(k, buf, msg)
		if err != nil {
			t.Fatal(err)
		}
		buf = buf[:n]

		// change mac, this must fail
		msg[len(msg)-8] ^= 0x23

		if _, err = Decrypt(k, buf, msg); err != ErrUnauthenticated {
			t.Fatal("wrong MAC value not detected")
		}

		// reset mac
		msg[len(msg)-8] ^= 0x23

		// tamper with message, this must fail
		msg[16+5] ^= 0x85

		if _, err = Decrypt(k, buf, msg); err != ErrUnauthenticated {
			t.Fatal("tampered message not detected")
		}

		// test decryption
		p := make([]byte, len(tv.ciphertext))
		n, err = Decrypt(k, p, tv.ciphertext)
		if err != nil {
			t.Fatal(err)
		}
		p = p[:n]

		if !bytes.Equal(p, tv.plaintext) {
			t.Fatalf("wrong plaintext: expected %q but got %q\n", tv.plaintext, p)
		}
	}
}
