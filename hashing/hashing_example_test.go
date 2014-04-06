package hashing_test

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"strings"

	"github.com/fd0/khepri/hashing"
)

func ExampleReader() {
	str := "foobar"
	reader := hashing.NewReader(strings.NewReader(str), md5.New)
	buf := make([]byte, len(str))

	reader.Read(buf)

	fmt.Printf("hash for %q is %02x", str, reader.Hash())
	// Output: hash for "foobar" is 3858f62230ac3c915f300c664312c63f
}

func ExampleWriter() {
	str := "foobar"
	var buf bytes.Buffer

	writer := hashing.NewWriter(&buf, sha1.New)
	writer.Write([]byte(str))

	fmt.Printf("hash for %q is %02x", str, writer.Hash())
	// Output: hash for "foobar" is 8843d7f92416211de9ebb963ff4ce28125932878
}
