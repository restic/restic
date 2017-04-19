// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package knownhosts implements a parser for the OpenSSH
// known_hosts host key database.
package knownhosts

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

// See the sshd manpage
// (http://man.openbsd.org/sshd#SSH_KNOWN_HOSTS_FILE_FORMAT) for
// background.

type addr struct{ host, port string }

func (a *addr) String() string {
	return a.host + ":" + a.port
}

func (a *addr) eq(b addr) bool {
	return a.host == b.host && a.port == b.port
}

type hostPattern struct {
	negate bool
	addr   addr
}

func (p *hostPattern) String() string {
	n := ""
	if p.negate {
		n = "!"
	}

	return n + p.addr.String()
}

// See
// https://android.googlesource.com/platform/external/openssh/+/ab28f5495c85297e7a597c1ba62e996416da7c7e/addrmatch.c
// The matching of * has no regard for separators, unlike filesystem globs
func wildcardMatch(pat []byte, str []byte) bool {
	for {
		if len(pat) == 0 {
			return len(str) == 0
		}
		if len(str) == 0 {
			return false
		}

		if pat[0] == '*' {
			if len(pat) == 1 {
				return true
			}

			for j := range str {
				if wildcardMatch(pat[1:], str[j:]) {
					return true
				}
			}
			return false
		}

		if pat[0] == '?' || pat[0] == str[0] {
			pat = pat[1:]
			str = str[1:]
		} else {
			return false
		}
	}
}

func (l *hostPattern) match(a addr) bool {
	return wildcardMatch([]byte(l.addr.host), []byte(a.host)) && l.addr.port == a.port
}

type keyDBLine struct {
	cert     bool
	patterns []*hostPattern
	knownKey KnownKey
}

func (l *keyDBLine) String() string {
	c := ""
	if l.cert {
		c = markerCert + " "
	}

	var ss []string
	for _, p := range l.patterns {
		ss = append(ss, p.String())
	}

	return c + strings.Join(ss, ",") + " " + serialize(l.knownKey.Key)
}

func serialize(k ssh.PublicKey) string {
	return k.Type() + " " + base64.StdEncoding.EncodeToString(k.Marshal())
}

func (l *keyDBLine) match(addrs []addr) bool {
	matched := false
	for _, p := range l.patterns {
		for _, a := range addrs {
			m := p.match(a)
			if p.negate {
				if m {
					return false
				} else {
					continue
				}
			}

			if m {
				matched = true
			}
		}
	}

	return matched
}

type hostKeyDB struct {
	// Serialized version of revoked keys
	revoked map[string]*KnownKey
	lines   []keyDBLine
}

func (db *hostKeyDB) String() string {
	var ls []string
	for _, k := range db.revoked {
		ls = append(ls, markerRevoked+" * "+serialize(k.Key))
	}
	for _, l := range db.lines {
		ls = append(ls, l.String())
	}
	return strings.Join(ls, "\n")
}

func newHostKeyDB() *hostKeyDB {
	db := &hostKeyDB{
		revoked: make(map[string]*KnownKey),
	}

	return db
}

func keyEq(a, b ssh.PublicKey) bool {
	return bytes.Equal(a.Marshal(), b.Marshal())
}

// IsAuthority can be used as a callback in ssh.CertChecker
func (db *hostKeyDB) IsAuthority(remote ssh.PublicKey) bool {
	for _, l := range db.lines {
		// TODO(hanwen): should we check the hostname against host pattern?
		if l.cert && keyEq(l.knownKey.Key, remote) {
			return true
		}
	}
	return false
}

// IsRevoked can be used as a callback in ssh.CertChecker
func (db *hostKeyDB) IsRevoked(key *ssh.Certificate) bool {
	_, ok := db.revoked[string(key.Marshal())]
	return ok
}

const markerCert = "@cert-authority"
const markerRevoked = "@revoked"

func nextWord(line []byte) (string, []byte) {
	i := bytes.IndexAny(line, "\t ")
	if i == -1 {
		return string(line), nil
	}

	return string(line[:i]), bytes.TrimSpace(line[i:])
}

func parseLine(line []byte) (marker string, pattern []string, key ssh.PublicKey, err error) {
	if w, next := nextWord(line); w == markerCert || w == markerRevoked {
		marker = w
		line = next
	}

	hostPart, line := nextWord(line)
	if len(line) == 0 {
		return "", nil, nil, errors.New("knownhosts: missing host pattern")
	}

	if len(hostPart) > 0 && hostPart[0] == '|' {
		return "", nil, nil, errors.New("knownhosts: hashed hostnames not implemented")
	}

	pattern = strings.Split(hostPart, ",")

	// ignore the keytype as it's in the key blob anyway.
	_, line = nextWord(line)
	if len(line) == 0 {
		return "", nil, nil, errors.New("knownhosts: missing key type pattern")
	}

	keyBlob, _ := nextWord(line)

	keyBytes, err := base64.StdEncoding.DecodeString(keyBlob)
	if err != nil {
		return "", nil, nil, err
	}
	key, err = ssh.ParsePublicKey(keyBytes)
	if err != nil {
		return "", nil, nil, err
	}

	return marker, pattern, key, nil
}

func (db *hostKeyDB) parseLine(line []byte, filename string, linenum int) error {
	marker, patterns, key, err := parseLine(line)
	if err != nil {
		return err
	}

	if marker == markerRevoked {
		db.revoked[string(key.Marshal())] = &KnownKey{
			Key:      key,
			Filename: filename,
			Line:     linenum,
		}

		return nil
	}

	entry := keyDBLine{
		cert: marker == markerCert,
		knownKey: KnownKey{
			Filename: filename,
			Line:     linenum,
			Key:      key,
		},
	}

	for _, p := range patterns {
		if len(p) == 0 {
			continue
		}

		var a addr
		var negate bool
		if p[0] == '!' {
			negate = true
			p = p[1:]
		}

		if len(p) == 0 {
			return errors.New("knownhosts: negation without following hostname")
		}

		if p[0] == '[' {
			a.host, a.port, err = net.SplitHostPort(p)
			if err != nil {
				return err
			}
		} else {
			a.host, a.port, err = net.SplitHostPort(p)
			if err != nil {
				a.host = p
				a.port = "22"
			}
		}

		entry.patterns = append(entry.patterns, &hostPattern{
			negate: negate,
			addr:   a,
		})
	}

	db.lines = append(db.lines, entry)
	return nil
}

// KnownKey represents a key declared in a known_hosts file.
type KnownKey struct {
	Key      ssh.PublicKey
	Filename string
	Line     int
}

func (k *KnownKey) String() string {
	return fmt.Sprintf("%s:%d: %s", k.Filename, k.Line, serialize(k.Key))
}

// KeyError is returned if we did not find the key in the host key
// database, or there was a mismatch.  Typically, in batch
// applications, this should be interpreted as failure. Interactive
// applications can offer an interactive prompt to the user.
type KeyError struct {
	// Want holds the accepted host keys. For each key algorithm,
	// there can be one hostkey.  If Want is empty, the host is
	// unknown. If Want is non-empty, there was a mismatch, which
	// can signify a MITM attack.
	Want []KnownKey
}

func (u *KeyError) Error() string {
	if len(u.Want) == 0 {
		return "knownhosts: key is unknown"
	}
	return "knownhosts: key mismatch"
}

// RevokedError is returned if we found a key that was revoked.
type RevokedError struct {
	Revoked KnownKey
}

func (r *RevokedError) Error() string {
	return "knownhosts: key is revoked"
}

// check checks a key against the host database. This should not be
// used for verifying certificates.
func (db *hostKeyDB) check(address string, remote net.Addr, remoteKey ssh.PublicKey) error {
	if revoked := db.revoked[string(remoteKey.Marshal())]; revoked != nil {
		return &RevokedError{Revoked: *revoked}
	}

	host, port, err := net.SplitHostPort(remote.String())
	if err != nil {
		return fmt.Errorf("knownhosts: SplitHostPort(%s): %v", remote, err)
	}

	addrs := []addr{
		{host, port},
	}

	if address != "" {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("knownhosts: SplitHostPort(%s): %v", address, err)
		}

		addrs = append(addrs, addr{host, port})
	}

	return db.checkAddrs(addrs, remoteKey)
}

// checkAddrs checks if we can find the given public key for any of
// the given addresses.  If we only find an entry for the IP address,
// or only the hostname, then this still succeeds.
func (db *hostKeyDB) checkAddrs(addrs []addr, remoteKey ssh.PublicKey) error {
	// TODO(hanwen): are these the right semantics? What if there
	// is just a key for the IP address, but not for the
	// hostname?

	// Algorithm => key.
	knownKeys := map[string]KnownKey{}
	for _, l := range db.lines {
		if l.match(addrs) {
			typ := l.knownKey.Key.Type()
			if _, ok := knownKeys[typ]; !ok {
				knownKeys[typ] = l.knownKey
			}
		}
	}

	keyErr := &KeyError{}
	for _, v := range knownKeys {
		keyErr.Want = append(keyErr.Want, v)
	}

	// Unknown remote host.
	if len(knownKeys) == 0 {
		return keyErr
	}

	// If the remote host starts using a different, unknown key type, we
	// also interpret that as a mismatch.
	if known, ok := knownKeys[remoteKey.Type()]; !ok || !keyEq(known.Key, remoteKey) {
		return keyErr
	}

	return nil
}

// The Read function parses file contents.
func (db *hostKeyDB) Read(r io.Reader, filename string) error {
	scanner := bufio.NewScanner(r)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		if err := db.parseLine(line, filename, lineNum); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// New creates a host key callback from the given OpenSSH host key
// files. The returned callback is for use in
// ssh.ClientConfig.HostKeyCallback. Hostnames are ignored for
// certificates, ie. any certificate authority is assumed to be valid
// for all remote hosts.  Hashed hostnames are not supported.
func New(files ...string) (ssh.HostKeyCallback, error) {
	db := newHostKeyDB()
	for _, fn := range files {
		f, err := os.Open(fn)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if err := db.Read(f, fn); err != nil {
			return nil, err
		}
	}

	// TODO(hanwen): properly supporting certificates requires an
	// API change in the SSH library: IsAuthority should provide
	// the address too?

	var certChecker ssh.CertChecker
	certChecker.IsAuthority = db.IsAuthority
	certChecker.IsRevoked = db.IsRevoked
	certChecker.HostKeyFallback = db.check

	return certChecker.CheckHostKey, nil
}

// Line returns a line to add append to the known_hosts files.
func Line(addresses []string, key ssh.PublicKey) string {
	var trimmed []string
	for _, a := range addresses {
		host, port, err := net.SplitHostPort(a)
		if err != nil {
			host = a
			port = "22"
		}
		entry := host
		if port != "22" {
			entry = "[" + entry + "]:" + port
		} else if strings.Contains(host, ":") {
			entry = "[" + entry + "]"
		}

		trimmed = append(trimmed, entry)
	}

	return strings.Join(trimmed, ",") + " " + serialize(key)
}
