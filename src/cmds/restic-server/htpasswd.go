// +build go1.4

package main

/*
Copied from: github.com/bitly/oauth2_proxy

MIT License

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/csv"
	"io"
	"log"

	"restic/fs"
)

// lookup passwords in a htpasswd file
// The entries must have been created with -s for SHA encryption

// HtpasswdFile is a map for usernames to passwords.
type HtpasswdFile struct {
	Users map[string]string
}

// NewHtpasswdFromFile reads the users and passwords from a htpasswd
// file and returns them. If an error is encountered, it is returned, together
// with a nil-Pointer for the HtpasswdFile.
func NewHtpasswdFromFile(path string) (*HtpasswdFile, error) {
	r, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return NewHtpasswd(r)
}

// NewHtpasswd reads the users and passwords from a htpasswd
// datastream in file and returns them. If an error is encountered,
// it is returned, together with a nil-Pointer for the HtpasswdFile.
func NewHtpasswd(file io.Reader) (*HtpasswdFile, error) {
	cr := csv.NewReader(file)
	cr.Comma = ':'
	cr.Comment = '#'
	cr.TrimLeadingSpace = true

	records, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	h := &HtpasswdFile{Users: make(map[string]string)}
	for _, record := range records {
		h.Users[record[0]] = record[1]
	}
	return h, nil
}

// Validate returns true if password matches the stored password
// for user. If no password for user is stored, or the password
// is wrong, false is returned.
func (h *HtpasswdFile) Validate(user string, password string) bool {
	realPassword, exists := h.Users[user]
	if !exists {
		return false
	}
	if realPassword[:5] == "{SHA}" {
		d := sha1.New()
		d.Write([]byte(password))
		if realPassword[5:] == base64.StdEncoding.EncodeToString(d.Sum(nil)) {
			return true
		}
	} else {
		log.Printf("Invalid htpasswd entry for %s. Must be a SHA entry.", user)
	}
	return false
}
