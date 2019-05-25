// Package textfile allows reading files that contain text. It automatically
// detects and converts several encodings and removes Byte Order Marks (BOM).
package textfile

import (
	"bytes"
	"io/ioutil"
	"os"

	"github.com/restic/restic/internal/errors"

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

// Trying to read more than 512 MiB of text
// is a good indication of something going wrong.
// Especially `--exclude` and `--exclude-file` are nasty.
const MaxSaneTextfileSize = 512 * 1024 * 1024

// Read returns the contents of the file, converted to UTF-8, stripped of any BOM.
func Read(filename string) ([]byte, error) {
	// First, sanity check the file size:
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil {
		if size := fi.Size() + bytes.MinRead; size > MaxSaneTextfileSize {
			return nil, errors.Fatalf("Textfile %s is unreasonably large (%dB > %dB)", filename, size, MaxSaneTextfileSize)
		}
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return Decode(data)
}
