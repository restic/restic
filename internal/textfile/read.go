// Package textfile allows reading files that contain text. It automatically
// detects and converts several encodings and removes Byte Order Marks (BOM).
package textfile

import (
	"bytes"
	"os"

	"golang.org/x/text/encoding/unicode"
)

// All supported BOMs (Byte Order Marks)
var (
	bomUTF8              = []byte{0xef, 0xbb, 0xbf}
	bomUTF16BigEndian    = []byte{0xfe, 0xff}
	bomUTF16LittleEndian = []byte{0xff, 0xfe}
)

// Decode removes a byte order mark and converts the bytes to UTF-8.
func Decode(data []byte) ([]byte, error) {
	if bytes.HasPrefix(data, bomUTF8) {
		return data[len(bomUTF8):], nil
	}

	if !bytes.HasPrefix(data, bomUTF16BigEndian) && !bytes.HasPrefix(data, bomUTF16LittleEndian) {
		// no encoding specified, let's assume UTF-8
		return data, nil
	}

	// UseBom means automatic endianness selection
	e := unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
	return e.NewDecoder().Bytes(data)
}

// Read returns the contents of the file, converted to UTF-8, stripped of any BOM.
func Read(filename string) ([]byte, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return Decode(data)
}
