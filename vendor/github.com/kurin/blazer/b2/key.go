// Copyright 2018, Google
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package b2

import (
	"context"
	"errors"
	"io"
	"time"
)

// Key is a B2 application key.  A Key grants limited access on a global or
// per-bucket basis.
type Key struct {
	c *Client
	k beKeyInterface
}

// Capabilities returns the list of capabilites granted by this application
// key.
func (k *Key) Capabilities() []string { return k.k.caps() }

// Name returns the user-supplied name of this application key.  Key names are
// useless.
func (k *Key) Name() string { return k.k.name() }

// Expires returns the expiration date of this application key.
func (k *Key) Expires() time.Time { return k.k.expires() }

// Delete removes the key from B2.
func (k *Key) Delete(ctx context.Context) error { return k.k.del(ctx) }

// Secret returns the value that should be passed into NewClient().  It is only
// available on newly created keys; it is not available from ListKey
// operations.
func (k *Key) Secret() string { return k.k.secret() }

// ID returns the application key ID.  This, plus the secret, is necessary to
// authenticate to B2.
func (k *Key) ID() string { return k.k.id() }

type keyOptions struct {
	caps     []string
	prefix   string
	lifetime time.Duration
}

// KeyOption specifies desired properties for application keys.
type KeyOption func(*keyOptions)

// Lifetime requests a key with the given lifetime.
func Lifetime(d time.Duration) KeyOption {
	return func(k *keyOptions) {
		k.lifetime = d
	}
}

// Deadline requests a key that expires after the given date.
func Deadline(t time.Time) KeyOption {
	d := t.Sub(time.Now())
	return Lifetime(d)
}

// Capabilities requests a key with the given capability.
func Capabilities(caps ...string) KeyOption {
	return func(k *keyOptions) {
		k.caps = append(k.caps, caps...)
	}
}

// Prefix limits the requested application key to be valid only for objects
// that begin with prefix.  This can only be used when requesting an
// application key within a specific bucket.
func Prefix(prefix string) KeyOption {
	return func(k *keyOptions) {
		k.prefix = prefix
	}
}

// CreateKey creates a global application key that is valid for all buckets in
// this project.  The key's secret will only be accessible on the object
// returned from this call.
func (c *Client) CreateKey(ctx context.Context, name string, opts ...KeyOption) (*Key, error) {
	var ko keyOptions
	for _, o := range opts {
		o(&ko)
	}
	if ko.prefix != "" {
		return nil, errors.New("Prefix is not a valid option for global application keys")
	}
	ki, err := c.backend.createKey(ctx, name, ko.caps, ko.lifetime, "", "")
	if err != nil {
		return nil, err
	}
	return &Key{
		c: c,
		k: ki,
	}, nil
}

// ListKeys lists all the keys associated with this project.  It takes the
// maximum number of keys it should return in a call, as well as a cursor
// (which should be empty for the initial call).  It will return up to count
// keys, as well as the cursor for the next invocation.
//
// ListKeys returns io.EOF when there are no more keys, although it may do so
// concurrently with the final set of keys.
func (c *Client) ListKeys(ctx context.Context, count int, cursor string) ([]*Key, string, error) {
	ks, next, err := c.backend.listKeys(ctx, count, cursor)
	if err != nil {
		return nil, "", err
	}
	if len(ks) == 0 {
		return nil, "", io.EOF
	}
	var keys []*Key
	for _, k := range ks {
		keys = append(keys, &Key{
			c: c,
			k: k,
		})
	}
	return keys, next, nil
}

// CreateKey creates a scoped application key that is valid only for this bucket.
func (b *Bucket) CreateKey(ctx context.Context, name string, opts ...KeyOption) (*Key, error) {
	var ko keyOptions
	for _, o := range opts {
		o(&ko)
	}
	ki, err := b.r.createKey(ctx, name, ko.caps, ko.lifetime, b.b.id(), ko.prefix)
	if err != nil {
		return nil, err
	}
	return &Key{
		c: b.c,
		k: ki,
	}, nil
}
