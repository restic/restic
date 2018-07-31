// Copyright 2016, Google
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
	"io"
	"math/rand"
	"time"
)

// This file wraps the baseline interfaces with backoff and retry semantics.

type beRootInterface interface {
	backoff(error) time.Duration
	reauth(error) bool
	transient(error) bool
	reupload(error) bool
	authorizeAccount(context.Context, string, string, clientOptions) error
	reauthorizeAccount(context.Context) error
	createBucket(ctx context.Context, name, btype string, info map[string]string, rules []LifecycleRule) (beBucketInterface, error)
	listBuckets(context.Context) ([]beBucketInterface, error)
	createKey(context.Context, string, []string, time.Duration, string, string) (beKeyInterface, error)
	listKeys(context.Context, int, string) ([]beKeyInterface, string, error)
}

type beRoot struct {
	account, key string
	b2i          b2RootInterface
	options      clientOptions
}

type beBucketInterface interface {
	name() string
	btype() BucketType
	attrs() *BucketAttrs
	id() string
	updateBucket(context.Context, *BucketAttrs) error
	deleteBucket(context.Context) error
	getUploadURL(context.Context) (beURLInterface, error)
	startLargeFile(ctx context.Context, name, contentType string, info map[string]string) (beLargeFileInterface, error)
	listFileNames(context.Context, int, string, string, string) ([]beFileInterface, string, error)
	listFileVersions(context.Context, int, string, string, string, string) ([]beFileInterface, string, string, error)
	listUnfinishedLargeFiles(context.Context, int, string) ([]beFileInterface, string, error)
	downloadFileByName(context.Context, string, int64, int64) (beFileReaderInterface, error)
	hideFile(context.Context, string) (beFileInterface, error)
	getDownloadAuthorization(context.Context, string, time.Duration, string) (string, error)
	baseURL() string
	file(string, string) beFileInterface
}

type beBucket struct {
	b2bucket b2BucketInterface
	ri       beRootInterface
}

type beURLInterface interface {
	uploadFile(context.Context, readResetter, int, string, string, string, map[string]string) (beFileInterface, error)
}

type beURL struct {
	b2url b2URLInterface
	ri    beRootInterface
}

type beFileInterface interface {
	name() string
	size() int64
	timestamp() time.Time
	status() string
	deleteFileVersion(context.Context) error
	getFileInfo(context.Context) (beFileInfoInterface, error)
	listParts(context.Context, int, int) ([]beFilePartInterface, int, error)
	compileParts(int64, map[int]string) beLargeFileInterface
}

type beFile struct {
	b2file b2FileInterface
	url    beURLInterface
	ri     beRootInterface
}

type beLargeFileInterface interface {
	finishLargeFile(context.Context) (beFileInterface, error)
	getUploadPartURL(context.Context) (beFileChunkInterface, error)
}

type beLargeFile struct {
	b2largeFile b2LargeFileInterface
	ri          beRootInterface
}

type beFileChunkInterface interface {
	reload(context.Context) error
	uploadPart(context.Context, readResetter, string, int, int) (int, error)
}

type beFileChunk struct {
	b2fileChunk b2FileChunkInterface
	ri          beRootInterface
}

type beFileReaderInterface interface {
	io.ReadCloser
	stats() (int, string, string, map[string]string)
	id() string
}

type beFileReader struct {
	b2fileReader b2FileReaderInterface
	ri           beRootInterface
}

type beFileInfoInterface interface {
	stats() (string, string, int64, string, map[string]string, string, time.Time)
}

type beFilePartInterface interface {
	number() int
	sha1() string
	size() int64
}

type beFilePart struct {
	b2filePart b2FilePartInterface
	ri         beRootInterface
}

type beFileInfo struct {
	name   string
	sha    string
	size   int64
	ct     string
	info   map[string]string
	status string
	stamp  time.Time
}

type beKeyInterface interface {
	del(context.Context) error
	caps() []string
	name() string
	expires() time.Time
	secret() string
	id() string
}

type beKey struct {
	b2i beRootInterface
	k   b2KeyInterface
}

func (r *beRoot) backoff(err error) time.Duration { return r.b2i.backoff(err) }
func (r *beRoot) reauth(err error) bool           { return r.b2i.reauth(err) }
func (r *beRoot) reupload(err error) bool         { return r.b2i.reupload(err) }
func (r *beRoot) transient(err error) bool        { return r.b2i.transient(err) }

func (r *beRoot) authorizeAccount(ctx context.Context, account, key string, c clientOptions) error {
	f := func() error {
		if err := r.b2i.authorizeAccount(ctx, account, key, c); err != nil {
			return err
		}
		r.account = account
		r.key = key
		r.options = c
		return nil
	}
	return withBackoff(ctx, r, f)
}

func (r *beRoot) reauthorizeAccount(ctx context.Context) error {
	return r.authorizeAccount(ctx, r.account, r.key, r.options)
}

func (r *beRoot) createBucket(ctx context.Context, name, btype string, info map[string]string, rules []LifecycleRule) (beBucketInterface, error) {
	var bi beBucketInterface
	f := func() error {
		g := func() error {
			bucket, err := r.b2i.createBucket(ctx, name, btype, info, rules)
			if err != nil {
				return err
			}
			bi = &beBucket{
				b2bucket: bucket,
				ri:       r,
			}
			return nil
		}
		return withReauth(ctx, r, g)
	}
	if err := withBackoff(ctx, r, f); err != nil {
		return nil, err
	}
	return bi, nil
}

func (r *beRoot) listBuckets(ctx context.Context) ([]beBucketInterface, error) {
	var buckets []beBucketInterface
	f := func() error {
		g := func() error {
			bs, err := r.b2i.listBuckets(ctx)
			if err != nil {
				return err
			}
			for _, b := range bs {
				buckets = append(buckets, &beBucket{
					b2bucket: b,
					ri:       r,
				})
			}
			return nil
		}
		return withReauth(ctx, r, g)
	}
	if err := withBackoff(ctx, r, f); err != nil {
		return nil, err
	}
	return buckets, nil
}

func (r *beRoot) createKey(ctx context.Context, name string, caps []string, valid time.Duration, bucketID string, prefix string) (beKeyInterface, error) {
	var k *beKey
	f := func() error {
		g := func() error {
			got, err := r.b2i.createKey(ctx, name, caps, valid, bucketID, prefix)
			if err != nil {
				return err
			}
			k = &beKey{
				b2i: r,
				k:   got,
			}
			return nil
		}
		return withReauth(ctx, r, g)
	}
	if err := withBackoff(ctx, r, f); err != nil {
		return nil, err
	}
	return k, nil
}

func (r *beRoot) listKeys(ctx context.Context, max int, next string) ([]beKeyInterface, string, error) {
	var keys []beKeyInterface
	var cur string
	f := func() error {
		g := func() error {
			got, n, err := r.b2i.listKeys(ctx, max, next)
			if err != nil {
				return err
			}
			cur = n
			for _, g := range got {
				keys = append(keys, &beKey{
					b2i: r,
					k:   g,
				})
			}
			return nil
		}
		return withReauth(ctx, r, g)
	}
	if err := withBackoff(ctx, r, f); err != nil {
		return nil, "", err
	}
	return keys, cur, nil
}

func (b *beBucket) name() string        { return b.b2bucket.name() }
func (b *beBucket) btype() BucketType   { return BucketType(b.b2bucket.btype()) }
func (b *beBucket) attrs() *BucketAttrs { return b.b2bucket.attrs() }
func (b *beBucket) id() string          { return b.b2bucket.id() }

func (b *beBucket) updateBucket(ctx context.Context, attrs *BucketAttrs) error {
	f := func() error {
		g := func() error {
			return b.b2bucket.updateBucket(ctx, attrs)
		}
		return withReauth(ctx, b.ri, g)
	}
	return withBackoff(ctx, b.ri, f)
}

func (b *beBucket) deleteBucket(ctx context.Context) error {
	f := func() error {
		g := func() error {
			return b.b2bucket.deleteBucket(ctx)
		}
		return withReauth(ctx, b.ri, g)
	}
	return withBackoff(ctx, b.ri, f)
}

func (b *beBucket) getUploadURL(ctx context.Context) (beURLInterface, error) {
	var url beURLInterface
	f := func() error {
		g := func() error {
			u, err := b.b2bucket.getUploadURL(ctx)
			if err != nil {
				return err
			}
			url = &beURL{
				b2url: u,
				ri:    b.ri,
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, err
	}
	return url, nil
}

func (b *beBucket) startLargeFile(ctx context.Context, name, ct string, info map[string]string) (beLargeFileInterface, error) {
	var file beLargeFileInterface
	f := func() error {
		g := func() error {
			f, err := b.b2bucket.startLargeFile(ctx, name, ct, info)
			if err != nil {
				return err
			}
			file = &beLargeFile{
				b2largeFile: f,
				ri:          b.ri,
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, err
	}
	return file, nil
}

func (b *beBucket) listFileNames(ctx context.Context, count int, continuation, prefix, delimiter string) ([]beFileInterface, string, error) {
	var cont string
	var files []beFileInterface
	f := func() error {
		g := func() error {
			fs, c, err := b.b2bucket.listFileNames(ctx, count, continuation, prefix, delimiter)
			if err != nil {
				return err
			}
			cont = c
			for _, f := range fs {
				files = append(files, &beFile{
					b2file: f,
					ri:     b.ri,
				})
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, "", err
	}
	return files, cont, nil
}

func (b *beBucket) listFileVersions(ctx context.Context, count int, nextName, nextID, prefix, delimiter string) ([]beFileInterface, string, string, error) {
	var name, id string
	var files []beFileInterface
	f := func() error {
		g := func() error {
			fs, n, d, err := b.b2bucket.listFileVersions(ctx, count, nextName, nextID, prefix, delimiter)
			if err != nil {
				return err
			}
			name = n
			id = d
			for _, f := range fs {
				files = append(files, &beFile{
					b2file: f,
					ri:     b.ri,
				})
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, "", "", err
	}
	return files, name, id, nil
}

func (b *beBucket) listUnfinishedLargeFiles(ctx context.Context, count int, continuation string) ([]beFileInterface, string, error) {
	var cont string
	var files []beFileInterface
	f := func() error {
		g := func() error {
			fs, c, err := b.b2bucket.listUnfinishedLargeFiles(ctx, count, continuation)
			if err != nil {
				return err
			}
			cont = c
			for _, f := range fs {
				files = append(files, &beFile{
					b2file: f,
					ri:     b.ri,
				})
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, "", err
	}
	return files, cont, nil
}

func (b *beBucket) downloadFileByName(ctx context.Context, name string, offset, size int64) (beFileReaderInterface, error) {
	var reader beFileReaderInterface
	f := func() error {
		g := func() error {
			fr, err := b.b2bucket.downloadFileByName(ctx, name, offset, size)
			if err != nil {
				return err
			}
			reader = &beFileReader{
				b2fileReader: fr,
				ri:           b.ri,
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, err
	}
	return reader, nil
}

func (b *beBucket) hideFile(ctx context.Context, name string) (beFileInterface, error) {
	var file beFileInterface
	f := func() error {
		g := func() error {
			f, err := b.b2bucket.hideFile(ctx, name)
			if err != nil {
				return err
			}
			file = &beFile{
				b2file: f,
				ri:     b.ri,
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, err
	}
	return file, nil
}

func (b *beBucket) getDownloadAuthorization(ctx context.Context, p string, v time.Duration, s string) (string, error) {
	var tok string
	f := func() error {
		g := func() error {
			t, err := b.b2bucket.getDownloadAuthorization(ctx, p, v, s)
			if err != nil {
				return err
			}
			tok = t
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return "", err
	}
	return tok, nil
}

func (b *beBucket) baseURL() string {
	return b.b2bucket.baseURL()
}

func (b *beBucket) file(id, name string) beFileInterface {
	return &beFile{
		b2file: b.b2bucket.file(id, name),
		ri:     b.ri,
	}
}

func (b *beURL) uploadFile(ctx context.Context, r readResetter, size int, name, ct, sha1 string, info map[string]string) (beFileInterface, error) {
	var file beFileInterface
	f := func() error {
		if err := r.Reset(); err != nil {
			return err
		}
		f, err := b.b2url.uploadFile(ctx, r, size, name, ct, sha1, info)
		if err != nil {
			return err
		}
		file = &beFile{
			b2file: f,
			url:    b,
			ri:     b.ri,
		}
		return nil
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, err
	}
	return file, nil
}

func (b *beFile) deleteFileVersion(ctx context.Context) error {
	f := func() error {
		g := func() error {
			return b.b2file.deleteFileVersion(ctx)
		}
		return withReauth(ctx, b.ri, g)
	}
	return withBackoff(ctx, b.ri, f)
}

func (b *beFile) size() int64 {
	return b.b2file.size()
}

func (b *beFile) name() string {
	return b.b2file.name()
}

func (b *beFile) timestamp() time.Time {
	return b.b2file.timestamp()
}

func (b *beFile) status() string {
	return b.b2file.status()
}

func (b *beFile) getFileInfo(ctx context.Context) (beFileInfoInterface, error) {
	var fileInfo beFileInfoInterface
	f := func() error {
		g := func() error {
			fi, err := b.b2file.getFileInfo(ctx)
			if err != nil {
				return err
			}
			name, sha, size, ct, info, status, stamp := fi.stats()
			fileInfo = &beFileInfo{
				name:   name,
				sha:    sha,
				size:   size,
				ct:     ct,
				info:   info,
				status: status,
				stamp:  stamp,
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, err
	}
	return fileInfo, nil
}

func (b *beFile) listParts(ctx context.Context, next, count int) ([]beFilePartInterface, int, error) {
	var fpi []beFilePartInterface
	var rnxt int
	f := func() error {
		g := func() error {
			ps, n, err := b.b2file.listParts(ctx, next, count)
			if err != nil {
				return err
			}
			rnxt = n
			for _, p := range ps {
				fpi = append(fpi, &beFilePart{
					b2filePart: p,
					ri:         b.ri,
				})
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, 0, err
	}
	return fpi, rnxt, nil
}

func (b *beFile) compileParts(size int64, seen map[int]string) beLargeFileInterface {
	return &beLargeFile{
		b2largeFile: b.b2file.compileParts(size, seen),
		ri:          b.ri,
	}
}

func (b *beLargeFile) getUploadPartURL(ctx context.Context) (beFileChunkInterface, error) {
	var chunk beFileChunkInterface
	f := func() error {
		g := func() error {
			fc, err := b.b2largeFile.getUploadPartURL(ctx)
			if err != nil {
				return err
			}
			chunk = &beFileChunk{
				b2fileChunk: fc,
				ri:          b.ri,
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, err
	}
	return chunk, nil
}

func (b *beLargeFile) finishLargeFile(ctx context.Context) (beFileInterface, error) {
	var file beFileInterface
	f := func() error {
		g := func() error {
			f, err := b.b2largeFile.finishLargeFile(ctx)
			if err != nil {
				return err
			}
			file = &beFile{
				b2file: f,
				ri:     b.ri,
			}
			return nil
		}
		return withReauth(ctx, b.ri, g)
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return nil, err
	}
	return file, nil
}

func (b *beFileChunk) reload(ctx context.Context) error {
	f := func() error {
		g := func() error {
			return b.b2fileChunk.reload(ctx)
		}
		return withReauth(ctx, b.ri, g)
	}
	return withBackoff(ctx, b.ri, f)
}

func (b *beFileChunk) uploadPart(ctx context.Context, r readResetter, sha1 string, size, index int) (int, error) {
	// no re-auth; pass it back up to the caller so they can get an new upload URI and token
	// TODO: we should handle that here probably
	var i int
	f := func() error {
		if err := r.Reset(); err != nil {
			return err
		}
		j, err := b.b2fileChunk.uploadPart(ctx, r, sha1, size, index)
		if err != nil {
			return err
		}
		i = j
		return nil
	}
	if err := withBackoff(ctx, b.ri, f); err != nil {
		return 0, err
	}
	return i, nil
}

func (b *beFileReader) Read(p []byte) (int, error) {
	return b.b2fileReader.Read(p)
}

func (b *beFileReader) Close() error {
	return b.b2fileReader.Close()
}

func (b *beFileReader) stats() (int, string, string, map[string]string) {
	return b.b2fileReader.stats()
}

func (b *beFileReader) id() string { return b.b2fileReader.id() }

func (b *beFileInfo) stats() (string, string, int64, string, map[string]string, string, time.Time) {
	return b.name, b.sha, b.size, b.ct, b.info, b.status, b.stamp
}

func (b *beFilePart) number() int  { return b.b2filePart.number() }
func (b *beFilePart) sha1() string { return b.b2filePart.sha1() }
func (b *beFilePart) size() int64  { return b.b2filePart.size() }

func (b *beKey) del(ctx context.Context) error { return b.k.del(ctx) }
func (b *beKey) caps() []string                { return b.k.caps() }
func (b *beKey) name() string                  { return b.k.name() }
func (b *beKey) expires() time.Time            { return b.k.expires() }
func (b *beKey) secret() string                { return b.k.secret() }
func (b *beKey) id() string                    { return b.k.id() }

func jitter(d time.Duration) time.Duration {
	f := float64(d)
	f /= 50
	f += f * (rand.Float64() - 0.5)
	return time.Duration(f)
}

func getBackoff(d time.Duration) time.Duration {
	if d > 30*time.Second {
		return 30*time.Second + jitter(d)
	}
	return d*2 + jitter(d*2)
}

var after = time.After

func withBackoff(ctx context.Context, ri beRootInterface, f func() error) error {
	backoff := 500 * time.Millisecond
	for {
		err := f()
		if !ri.transient(err) {
			return err
		}
		bo := ri.backoff(err)
		if bo > 0 {
			backoff = bo
		} else {
			backoff = getBackoff(backoff)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-after(backoff):
		}
	}
}

func withReauth(ctx context.Context, ri beRootInterface, f func() error) error {
	err := f()
	if ri.reauth(err) {
		if err := ri.reauthorizeAccount(ctx); err != nil {
			return err
		}
		err = f()
	}
	return err
}
