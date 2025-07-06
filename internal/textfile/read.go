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

	// Only decode as UTF-16 if the data length is reasonable for UTF-16 and
	// doesn't contain obvious ASCII patterns that would indicate false BOM detection
	if len(data) < 4 {
		// Too short to be valid UTF-16 with BOM, treat as UTF-8
		return data, nil
	}

	// UseBom means automatic endianness selection
	e := unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
	decoded, err := e.NewDecoder().Bytes(data)
	if err != nil {
		// UTF-16 decoding failed, fallback to treating as UTF-8
		return data, nil
	}

	// Check if the decoded result contains mostly printable ASCII characters
	// which might indicate false BOM detection on an ASCII file
	if isLikelyASCII(decoded) && len(decoded) > 10 {
		// This looks like ASCII text that was incorrectly detected as UTF-16,
		// return the original data instead
		return data, nil
	}

	return decoded, nil
}

// isLikelyASCII checks if the decoded text looks like normal ASCII text
// that might have been incorrectly processed as UTF-16
func isLikelyASCII(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	
	printableCount := 0
	for _, b := range data {
		if b >= 32 && b <= 126 || b == '\n' || b == '\r' || b == '\t' {
			printableCount++
		}
	}
	
	// If more than 80% of characters are printable ASCII, it's likely ASCII
	return float64(printableCount)/float64(len(data)) > 0.8
}

// Read returns the contents of the file, converted to UTF-8, stripped of any BOM.
func Read(filename string) ([]byte, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return Decode(data)
}
