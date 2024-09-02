package smb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/limiter"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

// Parts of this code have been adapted from Rclone (https://github.com/rclone)
// Copyright (C) 2012 by Nick Craig-Wood http://www.craig-wood.com/nick/

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// SMB is a backend which stores the data on an SMB share.
type SMB struct {
	sessions int32
	poolMu   sync.Mutex
	pool     []*conn
	drain    *time.Timer // used to drain the pool when we stop using the connections

	Config
	layout.Layout
	util.Modes
}

// Ensure statically that *SMB implements backend.Backend interface.
var _ backend.Backend = &SMB{}

// NewFactory returns a new SMB backend factory.
func NewFactory() location.Factory {
	return location.NewLimitedBackendFactory("smb", ParseConfig, location.NoPassword, limiter.WrapBackendConstructor(Create), limiter.WrapBackendConstructor(Open))
}

// open initializes a new SMB backend.
func open(cfg Config) (*SMB, error) {
	l := layout.NewDefaultLayout(cfg.Path, filepath.Join)

	b := &SMB{
		Config: cfg,
		Layout: l,
	}

	debug.Log("open, config %#v", cfg)

	// set the pool drainer timer going
	if b.Config.IdleTimeout > 0 {
		b.drain = time.AfterFunc(b.Config.IdleTimeout, func() { _ = b.drainPool() })
	}

	cn, err := b.getConnection(b.ShareName)
	if err != nil {
		return nil, err
	}
	defer b.putConnection(cn)

	b.Modes = util.DeriveModesFromStat(l, cn.smbShare.Stat)
	return b, nil
}

// Open opens the SMB backend as specified by the config.
func Open(_ context.Context, cfg Config) (*SMB, error) {
	debug.Log("open smb backend at %v (share %q)", cfg.Path, cfg.ShareName)
	return open(cfg)
}

// Create creates all the necessary files and directories for a new SMB backend.
func Create(_ context.Context, cfg Config) (*SMB, error) {
	debug.Log("create smb backend at %v (share %q)", cfg.Path, cfg.ShareName)
	b, err := open(cfg)
	if err != nil {
		return nil, err
	}
	cn, err := b.getConnection(cfg.ShareName)
	if err != nil {
		return b, err
	}
	defer b.putConnection(cn)

	err = util.Create(b.Filename(backend.Handle{Type: backend.ConfigFile}), b.Modes.Dir, b.Paths(), cn.smbShare.Lstat, cn.smbShare.MkdirAll)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Connections returns the number of configured connections.
func (b *SMB) Connections() uint {
	return b.Config.Connections
}

// Hasher returns a hash function for calculating a content hash for the backend.
func (b *SMB) Hasher() hash.Hash {
	return nil
}

// HasAtomicReplace returns whether Save() can atomically replace files.
func (b *SMB) HasAtomicReplace() bool {
	return true
}

// IsNotExist returns true if the error is caused by a non-existing file.
func (b *SMB) IsNotExist(err error) bool {
	return util.IsNotExist(err)
}

// IsPermanentError returns true if the error is permanent.
func (b *SMB) IsPermanentError(err error) bool {
	return util.IsPermanentError(err)
}

// Save stores data in the backend at the handle.
func (b *SMB) Save(_ context.Context, h backend.Handle, rd backend.RewindReader) error {
	b.addSession() // Show session in use
	defer b.removeSession()

	cn, err := b.getConnection(b.ShareName)
	if err != nil {
		return err
	}
	defer b.putConnection(cn)

	fileName := b.Filename(h)
	// For SMB, we use full path to the file for the temp file name
	tmpFilename := fileName + "-restic-temp-" + tempSuffix()

	saveOptions := util.SaveOptions{
		OpenTempFile: func(_, name string) (util.File, error) {
			return cn.smbShare.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		},
		MkDir: func(dir string) error {
			return cn.smbShare.MkdirAll(dir, b.Modes.Dir)
		},
		Remove:          cn.smbShare.Remove,
		IsMacENOTTY:     func(error) bool { return false },
		Rename:          cn.smbShare.Rename,
		FsyncDir:        func(_ string) error { return nil },
		SetFileReadonly: func(f string) error { return cn.setFileReadonly(f, b.Modes.File) },
		DirMode:         b.Modes.Dir,
		FileMode:        b.Modes.File,
	}

	return util.SaveWithOptions(fileName, tmpFilename, rd, saveOptions)
}

// setFileReadonly sets the file to read-only mode.
func (cn *conn) setFileReadonly(f string, mode os.FileMode) error {
	return cn.smbShare.Chmod(f, mode&^0222)
}

// Load runs fn with a reader that yields the contents of the file at h at the given offset.
func (b *SMB) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return util.DefaultLoad(ctx, h, length, offset, b.openReader, fn)
}

func (b *SMB) openReader(_ context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	b.addSession() // Show session in use
	defer b.removeSession()
	cn, err := b.getConnection(b.ShareName)
	if err != nil {
		return nil, err
	}
	defer b.putConnection(cn)
	openFile := func(name string) (util.File, error) {
		return cn.smbShare.Open(name)
	}
	return util.OpenReader(openFile, b.Filename(h), length, offset)
}

// Stat returns information about a blob.
func (b *SMB) Stat(_ context.Context, h backend.Handle) (backend.FileInfo, error) {
	cn, err := b.getConnection(b.ShareName)
	if err != nil {
		return backend.FileInfo{}, err
	}
	defer b.putConnection(cn)
	return util.Stat(cn.smbShare.Stat, b.Filename(h), h.Name)
}

// Remove removes the blob with the given name and type.
func (b *SMB) Remove(_ context.Context, h backend.Handle) error {
	cn, err := b.getConnection(b.ShareName)
	if err != nil {
		return err
	}
	defer b.putConnection(cn)
	return util.Remove(b.Filename(h), cn.smbShare.Chmod, cn.smbShare.Remove)
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (b *SMB) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	cn, err := b.getConnection(b.ShareName)
	if err != nil {
		return err
	}
	defer b.putConnection(cn)
	openFunc := func(name string) (util.File, error) {
		return cn.smbShare.Open(name)
	}
	basedir, subdirs := b.Basedir(t)
	return util.List(ctx, basedir, subdirs, openFunc, fn)
}

// Delete removes the repository and all files.
func (b *SMB) Delete(_ context.Context) error {
	cn, err := b.getConnection(b.ShareName)
	if err != nil {
		return err
	}
	defer b.putConnection(cn)
	return cn.smbShare.RemoveAll(path.Join(b.ShareName, b.Path))
}

// Close closes all open files.
func (b *SMB) Close() error {
	return b.drainPool()
}

// tempSuffix generates a random string suffix that should be sufficiently long
// to avoid accidental conflicts.
func tempSuffix() string {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic(errors.Wrap(err, "failed to generate random suffix"))
	}
	return hex.EncodeToString(nonce[:])
}
