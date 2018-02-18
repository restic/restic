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

package base

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kurin/blazer/x/transport"

	"context"
)

const (
	apiID  = "B2_ACCOUNT_ID"
	apiKey = "B2_SECRET_KEY"
)

const (
	bucketName    = "base-tests"
	smallFileName = "TeenyTiny"
	largeFileName = "BigBytes"
)

type zReader struct{}

func (zReader) Read(p []byte) (int, error) {
	return len(p), nil
}

func TestStorage(t *testing.T) {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
	}
	ctx := context.Background()

	// b2_authorize_account
	b2, err := AuthorizeAccount(ctx, id, key, UserAgent("blazer-base-test"))
	if err != nil {
		t.Fatal(err)
	}

	// b2_create_bucket
	infoKey := "key"
	infoVal := "val"
	m := map[string]string{infoKey: infoVal}
	rules := []LifecycleRule{
		{
			Prefix:             "what/",
			DaysNewUntilHidden: 5,
		},
	}
	bname := id + "-" + bucketName
	bucket, err := b2.CreateBucket(ctx, bname, "", m, rules)
	if err != nil {
		t.Fatal(err)
	}
	if bucket.Info[infoKey] != infoVal {
		t.Errorf("%s: bucketInfo[%q] got %q, want %q", bucket.Name, infoKey, bucket.Info[infoKey], infoVal)
	}
	if len(bucket.LifecycleRules) != 1 {
		t.Errorf("%s: lifecycle rules: got %d rules, wanted 1", bucket.Name, len(bucket.LifecycleRules))
	}

	defer func() {
		// b2_delete_bucket
		if err := bucket.DeleteBucket(ctx); err != nil {
			t.Error(err)
		}
	}()

	// b2_update_bucket
	bucket.Info["new"] = "yay"
	bucket.LifecycleRules = nil // Unset options should be a noop.
	newBucket, err := bucket.Update(ctx)
	if err != nil {
		t.Errorf("%s: update bucket: %v", bucket.Name, err)
		return
	}
	bucket = newBucket
	if bucket.Info["new"] != "yay" {
		t.Errorf("%s: info key \"new\": got %s, want \"yay\"", bucket.Name, bucket.Info["new"])
	}
	if len(bucket.LifecycleRules) != 1 {
		t.Errorf("%s: lifecycle rules: got %d rules, wanted 1", bucket.Name, len(bucket.LifecycleRules))
	}

	// b2_list_buckets
	buckets, err := b2.ListBuckets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, bucket := range buckets {
		if bucket.Name == bname {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("%s: new bucket not found", bname)
	}

	// b2_get_upload_url
	ue, err := bucket.GetUploadURL(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// b2_upload_file
	smallFile := io.LimitReader(zReader{}, 1024*50) // 50k
	hash := sha1.New()
	buf := &bytes.Buffer{}
	w := io.MultiWriter(hash, buf)
	if _, err := io.Copy(w, smallFile); err != nil {
		t.Error(err)
	}
	smallSHA1 := fmt.Sprintf("%x", hash.Sum(nil))
	smallInfoMap := map[string]string{
		"one": "1",
		"two": "2",
	}
	file, err := ue.UploadFile(ctx, buf, buf.Len(), smallFileName, "application/octet-stream", smallSHA1, smallInfoMap)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		// b2_delete_file_version
		if err := file.DeleteFileVersion(ctx); err != nil {
			t.Error(err)
		}
	}()

	// b2_start_large_file
	largeInfoMap := map[string]string{
		"one_billion":  "1e9",
		"two_trillion": "2eSomething, I guess 2e12",
	}
	lf, err := bucket.StartLargeFile(ctx, largeFileName, "application/octet-stream", largeInfoMap)
	if err != nil {
		t.Fatal(err)
	}

	// b2_get_upload_part_url
	fc, err := lf.GetUploadPartURL(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// b2_upload_part
	largeFile := io.LimitReader(zReader{}, 10e6) // 10M
	for i := 0; i < 2; i++ {
		r := io.LimitReader(largeFile, 5e6) // 5M
		hash := sha1.New()
		buf := &bytes.Buffer{}
		w := io.MultiWriter(hash, buf)
		if _, err := io.Copy(w, r); err != nil {
			t.Error(err)
		}
		if _, err := fc.UploadPart(ctx, buf, fmt.Sprintf("%x", hash.Sum(nil)), buf.Len(), i+1); err != nil {
			t.Error(err)
		}
	}

	// b2_finish_large_file
	lfile, err := lf.FinishLargeFile(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// b2_get_file_info
	smallInfo, err := file.GetFileInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	compareFileAndInfo(t, smallInfo, smallFileName, smallSHA1, smallInfoMap)
	largeInfo, err := lfile.GetFileInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	compareFileAndInfo(t, largeInfo, largeFileName, "none", largeInfoMap)

	defer func() {
		if err := lfile.DeleteFileVersion(ctx); err != nil {
			t.Error(err)
		}
	}()

	clf, err := bucket.StartLargeFile(ctx, largeFileName, "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	// b2_cancel_large_file
	if err := clf.CancelLargeFile(ctx); err != nil {
		t.Fatal(err)
	}

	// b2_list_file_names
	files, _, err := bucket.ListFileNames(ctx, 100, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(files), files)
	}

	// b2_download_file_by_name
	fr, err := bucket.DownloadFileByName(ctx, smallFileName, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fr.SHA1 != smallSHA1 {
		t.Errorf("small file SHAs don't match: got %q, want %q", fr.SHA1, smallSHA1)
	}
	lbuf := &bytes.Buffer{}
	if _, err := io.Copy(lbuf, fr); err != nil {
		t.Fatal(err)
	}
	if lbuf.Len() != fr.ContentLength {
		t.Errorf("small file retreived lengths don't match: got %d, want %d", lbuf.Len(), fr.ContentLength)
	}

	// b2_hide_file
	hf, err := bucket.HideFile(ctx, smallFileName)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := hf.DeleteFileVersion(ctx); err != nil {
			t.Error(err)
		}
	}()

	// b2_list_file_versions
	files, _, _, err = bucket.ListFileVersions(ctx, 100, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(files), files)
	}

	// b2_get_download_authorization
	if _, err := bucket.GetDownloadAuthorization(ctx, "foo/", 24*time.Hour); err != nil {
		t.Errorf("failed to get download auth token: %v", err)
	}
}

func TestUploadAuthAfterConnectionHang(t *testing.T) {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
	}
	ctx := context.Background()

	hung := make(chan struct{})

	// An http.RoundTripper that dies after sending ~10k bytes.
	hang := func() {
		close(hung)
		select {}
	}
	tport := transport.WithFailures(nil, transport.AfterNBytes(10000, hang))

	b2, err := AuthorizeAccount(ctx, id, key, Transport(tport))
	if err != nil {
		t.Fatal(err)
	}
	bname := id + "-" + bucketName
	bucket, err := b2.CreateBucket(ctx, bname, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := bucket.DeleteBucket(ctx); err != nil {
			t.Error(err)
		}
	}()
	ue, err := bucket.GetUploadURL(ctx)
	if err != nil {
		t.Fatal(err)
	}

	smallFile := io.LimitReader(zReader{}, 1024*50) // 50k
	hash := sha1.New()
	buf := &bytes.Buffer{}
	w := io.MultiWriter(hash, buf)
	if _, err := io.Copy(w, smallFile); err != nil {
		t.Error(err)
	}
	smallSHA1 := fmt.Sprintf("%x", hash.Sum(nil))

	go func() {
		ue.UploadFile(ctx, buf, buf.Len(), smallFileName, "application/octet-stream", smallSHA1, nil)
		t.Fatal("this ought not to be reachable")
	}()

	<-hung

	// Do the whole thing again with the same upload auth, before the remote end
	// notices we're gone.
	smallFile = io.LimitReader(zReader{}, 1024*50) // 50k again
	buf.Reset()
	if _, err := io.Copy(buf, smallFile); err != nil {
		t.Error(err)
	}
	file, err := ue.UploadFile(ctx, buf, buf.Len(), smallFileName, "application/octet-stream", smallSHA1, nil)
	if err == nil {
		t.Error("expected an error, got none")
		if err := file.DeleteFileVersion(ctx); err != nil {
			t.Error(err)
		}
	}
	if Action(err) != AttemptNewUpload {
		t.Errorf("Action(%v): got %v, want AttemptNewUpload", err, Action(err))
	}
}

func TestCancelledContextCancelsHTTPRequest(t *testing.T) {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
	}
	ctx := context.Background()

	tport := transport.WithFailures(nil, transport.MatchPathSubstring("b2_upload_file"), transport.FailureRate(1), transport.Stall(2*time.Second))

	b2, err := AuthorizeAccount(ctx, id, key, Transport(tport))
	if err != nil {
		t.Fatal(err)
	}
	bname := id + "-" + bucketName
	bucket, err := b2.CreateBucket(ctx, bname, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := bucket.DeleteBucket(ctx); err != nil {
			t.Error(err)
		}
	}()
	ue, err := bucket.GetUploadURL(ctx)
	if err != nil {
		t.Fatal(err)
	}

	smallFile := io.LimitReader(zReader{}, 1024*50) // 50k
	hash := sha1.New()
	buf := &bytes.Buffer{}
	w := io.MultiWriter(hash, buf)
	if _, err := io.Copy(w, smallFile); err != nil {
		t.Error(err)
	}
	smallSHA1 := fmt.Sprintf("%x", hash.Sum(nil))
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(1)
		cancel()
	}()
	if _, err := ue.UploadFile(cctx, buf, buf.Len(), smallFileName, "application/octet-stream", smallSHA1, nil); err != context.Canceled {
		t.Errorf("expected canceled context, but got %v", err)
	}
}

func TestDeadlineExceededContextCancelsHTTPRequest(t *testing.T) {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
	}
	ctx := context.Background()

	tport := transport.WithFailures(nil, transport.MatchPathSubstring("b2_upload_file"), transport.FailureRate(1), transport.Stall(2*time.Second))
	b2, err := AuthorizeAccount(ctx, id, key, Transport(tport))
	if err != nil {
		t.Fatal(err)
	}
	bname := id + "-" + bucketName
	bucket, err := b2.CreateBucket(ctx, bname, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := bucket.DeleteBucket(ctx); err != nil {
			t.Error(err)
		}
	}()
	ue, err := bucket.GetUploadURL(ctx)
	if err != nil {
		t.Fatal(err)
	}

	smallFile := io.LimitReader(zReader{}, 1024*50) // 50k
	hash := sha1.New()
	buf := &bytes.Buffer{}
	w := io.MultiWriter(hash, buf)
	if _, err := io.Copy(w, smallFile); err != nil {
		t.Error(err)
	}
	smallSHA1 := fmt.Sprintf("%x", hash.Sum(nil))
	cctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := ue.UploadFile(cctx, buf, buf.Len(), smallFileName, "application/octet-stream", smallSHA1, nil); err != context.DeadlineExceeded {
		t.Errorf("expected deadline exceeded error, but got %v", err)
	}
}

func compareFileAndInfo(t *testing.T, info *FileInfo, name, sha1 string, imap map[string]string) {
	if info.Name != name {
		t.Errorf("got %q, want %q", info.Name, name)
	}
	if info.SHA1 != sha1 {
		t.Errorf("got %q, want %q", info.SHA1, sha1)
	}
	if !reflect.DeepEqual(info.Info, imap) {
		t.Errorf("got %v, want %v", info.Info, imap)
	}
}

// from https://www.backblaze.com/b2/docs/string_encoding.html
var testCases = `[
  {"fullyEncoded": "%20", "minimallyEncoded": "+", "string": " "},
  {"fullyEncoded": "%21", "minimallyEncoded": "!", "string": "!"},
  {"fullyEncoded": "%22", "minimallyEncoded": "%22", "string": "\""},
  {"fullyEncoded": "%23", "minimallyEncoded": "%23", "string": "#"},
  {"fullyEncoded": "%24", "minimallyEncoded": "$", "string": "$"},
  {"fullyEncoded": "%25", "minimallyEncoded": "%25", "string": "%"},
  {"fullyEncoded": "%26", "minimallyEncoded": "%26", "string": "&"},
  {"fullyEncoded": "%27", "minimallyEncoded": "'", "string": "'"},
  {"fullyEncoded": "%28", "minimallyEncoded": "(", "string": "("},
  {"fullyEncoded": "%29", "minimallyEncoded": ")", "string": ")"},
  {"fullyEncoded": "%2A", "minimallyEncoded": "*", "string": "*"},
  {"fullyEncoded": "%2B", "minimallyEncoded": "%2B", "string": "+"},
  {"fullyEncoded": "%2C", "minimallyEncoded": "%2C", "string": ","},
  {"fullyEncoded": "%2D", "minimallyEncoded": "-", "string": "-"},
  {"fullyEncoded": "%2E", "minimallyEncoded": ".", "string": "."},
  {"fullyEncoded": "/", "minimallyEncoded": "/", "string": "/"},
  {"fullyEncoded": "%30", "minimallyEncoded": "0", "string": "0"},
  {"fullyEncoded": "%31", "minimallyEncoded": "1", "string": "1"},
  {"fullyEncoded": "%32", "minimallyEncoded": "2", "string": "2"},
  {"fullyEncoded": "%33", "minimallyEncoded": "3", "string": "3"},
  {"fullyEncoded": "%34", "minimallyEncoded": "4", "string": "4"},
  {"fullyEncoded": "%35", "minimallyEncoded": "5", "string": "5"},
  {"fullyEncoded": "%36", "minimallyEncoded": "6", "string": "6"},
  {"fullyEncoded": "%37", "minimallyEncoded": "7", "string": "7"},
  {"fullyEncoded": "%38", "minimallyEncoded": "8", "string": "8"},
  {"fullyEncoded": "%39", "minimallyEncoded": "9", "string": "9"},
  {"fullyEncoded": "%3A", "minimallyEncoded": ":", "string": ":"},
  {"fullyEncoded": "%3B", "minimallyEncoded": ";", "string": ";"},
  {"fullyEncoded": "%3C", "minimallyEncoded": "%3C", "string": "<"},
  {"fullyEncoded": "%3D", "minimallyEncoded": "=", "string": "="},
  {"fullyEncoded": "%3E", "minimallyEncoded": "%3E", "string": ">"},
  {"fullyEncoded": "%3F", "minimallyEncoded": "%3F", "string": "?"},
  {"fullyEncoded": "%40", "minimallyEncoded": "@", "string": "@"},
  {"fullyEncoded": "%41", "minimallyEncoded": "A", "string": "A"},
  {"fullyEncoded": "%42", "minimallyEncoded": "B", "string": "B"},
  {"fullyEncoded": "%43", "minimallyEncoded": "C", "string": "C"},
  {"fullyEncoded": "%44", "minimallyEncoded": "D", "string": "D"},
  {"fullyEncoded": "%45", "minimallyEncoded": "E", "string": "E"},
  {"fullyEncoded": "%46", "minimallyEncoded": "F", "string": "F"},
  {"fullyEncoded": "%47", "minimallyEncoded": "G", "string": "G"},
  {"fullyEncoded": "%48", "minimallyEncoded": "H", "string": "H"},
  {"fullyEncoded": "%49", "minimallyEncoded": "I", "string": "I"},
  {"fullyEncoded": "%4A", "minimallyEncoded": "J", "string": "J"},
  {"fullyEncoded": "%4B", "minimallyEncoded": "K", "string": "K"},
  {"fullyEncoded": "%4C", "minimallyEncoded": "L", "string": "L"},
  {"fullyEncoded": "%4D", "minimallyEncoded": "M", "string": "M"},
  {"fullyEncoded": "%4E", "minimallyEncoded": "N", "string": "N"},
  {"fullyEncoded": "%4F", "minimallyEncoded": "O", "string": "O"},
  {"fullyEncoded": "%50", "minimallyEncoded": "P", "string": "P"},
  {"fullyEncoded": "%51", "minimallyEncoded": "Q", "string": "Q"},
  {"fullyEncoded": "%52", "minimallyEncoded": "R", "string": "R"},
  {"fullyEncoded": "%53", "minimallyEncoded": "S", "string": "S"},
  {"fullyEncoded": "%54", "minimallyEncoded": "T", "string": "T"},
  {"fullyEncoded": "%55", "minimallyEncoded": "U", "string": "U"},
  {"fullyEncoded": "%56", "minimallyEncoded": "V", "string": "V"},
  {"fullyEncoded": "%57", "minimallyEncoded": "W", "string": "W"},
  {"fullyEncoded": "%58", "minimallyEncoded": "X", "string": "X"},
  {"fullyEncoded": "%59", "minimallyEncoded": "Y", "string": "Y"},
  {"fullyEncoded": "%5A", "minimallyEncoded": "Z", "string": "Z"},
  {"fullyEncoded": "%5B", "minimallyEncoded": "%5B", "string": "["},
  {"fullyEncoded": "%5C", "minimallyEncoded": "%5C", "string": "\\"},
  {"fullyEncoded": "%5D", "minimallyEncoded": "%5D", "string": "]"},
  {"fullyEncoded": "%5E", "minimallyEncoded": "%5E", "string": "^"},
  {"fullyEncoded": "%5F", "minimallyEncoded": "_", "string": "_"},
  {"fullyEncoded": "%60", "minimallyEncoded": "%60", "string": "` + "`" + `"},
  {"fullyEncoded": "%61", "minimallyEncoded": "a", "string": "a"},
  {"fullyEncoded": "%62", "minimallyEncoded": "b", "string": "b"},
  {"fullyEncoded": "%63", "minimallyEncoded": "c", "string": "c"},
  {"fullyEncoded": "%64", "minimallyEncoded": "d", "string": "d"},
  {"fullyEncoded": "%65", "minimallyEncoded": "e", "string": "e"},
  {"fullyEncoded": "%66", "minimallyEncoded": "f", "string": "f"},
  {"fullyEncoded": "%67", "minimallyEncoded": "g", "string": "g"},
  {"fullyEncoded": "%68", "minimallyEncoded": "h", "string": "h"},
  {"fullyEncoded": "%69", "minimallyEncoded": "i", "string": "i"},
  {"fullyEncoded": "%6A", "minimallyEncoded": "j", "string": "j"},
  {"fullyEncoded": "%6B", "minimallyEncoded": "k", "string": "k"},
  {"fullyEncoded": "%6C", "minimallyEncoded": "l", "string": "l"},
  {"fullyEncoded": "%6D", "minimallyEncoded": "m", "string": "m"},
  {"fullyEncoded": "%6E", "minimallyEncoded": "n", "string": "n"},
  {"fullyEncoded": "%6F", "minimallyEncoded": "o", "string": "o"},
  {"fullyEncoded": "%70", "minimallyEncoded": "p", "string": "p"},
  {"fullyEncoded": "%71", "minimallyEncoded": "q", "string": "q"},
  {"fullyEncoded": "%72", "minimallyEncoded": "r", "string": "r"},
  {"fullyEncoded": "%73", "minimallyEncoded": "s", "string": "s"},
  {"fullyEncoded": "%74", "minimallyEncoded": "t", "string": "t"},
  {"fullyEncoded": "%75", "minimallyEncoded": "u", "string": "u"},
  {"fullyEncoded": "%76", "minimallyEncoded": "v", "string": "v"},
  {"fullyEncoded": "%77", "minimallyEncoded": "w", "string": "w"},
  {"fullyEncoded": "%78", "minimallyEncoded": "x", "string": "x"},
  {"fullyEncoded": "%79", "minimallyEncoded": "y", "string": "y"},
  {"fullyEncoded": "%7A", "minimallyEncoded": "z", "string": "z"},
  {"fullyEncoded": "%7B", "minimallyEncoded": "%7B", "string": "{"},
  {"fullyEncoded": "%7C", "minimallyEncoded": "%7C", "string": "|"},
  {"fullyEncoded": "%7D", "minimallyEncoded": "%7D", "string": "}"},
  {"fullyEncoded": "%7E", "minimallyEncoded": "~", "string": "~"},
  {"fullyEncoded": "%7F", "minimallyEncoded": "%7F", "string": "\u007f"},
  {"fullyEncoded": "%E8%87%AA%E7%94%B1", "minimallyEncoded": "%E8%87%AA%E7%94%B1", "string": "\u81ea\u7531"},
  {"fullyEncoded": "%F0%90%90%80", "minimallyEncoded": "%F0%90%90%80", "string": "\ud801\udc00"}
]`

type testCase struct {
	Full string `json:"fullyEncoded"`
	Min  string `json:"minimallyEncoded"`
	Raw  string `json:"string"`
}

func TestEscapes(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(testCases))
	var tcs []testCase
	if err := dec.Decode(&tcs); err != nil {
		t.Fatal(err)
	}
	for _, tc := range tcs {
		en := escape(tc.Raw)
		if !(en == tc.Full || en == tc.Min) {
			t.Errorf("encode %q: got %q, want %q or %q", tc.Raw, en, tc.Min, tc.Full)
		}

		m, err := unescape(tc.Min)
		if err != nil {
			t.Errorf("decode %q: %v", tc.Min, err)
		}
		if m != tc.Raw {
			t.Errorf("decode %q: got %q, want %q", tc.Min, m, tc.Raw)
		}
		f, err := unescape(tc.Full)
		if err != nil {
			t.Errorf("decode %q: %v", tc.Full, err)
		}
		if f != tc.Raw {
			t.Errorf("decode %q: got %q, want %q", tc.Full, f, tc.Raw)
		}
	}
}

func TestUploadDownloadFilenameEscaping(t *testing.T) {
	filename := "file%foo.txt"

	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)

	if id == "" || key == "" {
		t.Skipf("B2_ACCOUNT_ID or B2_SECRET_KEY unset; skipping integration tests")
	}
	ctx := context.Background()

	// b2_authorize_account
	b2, err := AuthorizeAccount(ctx, id, key, UserAgent("blazer-base-test"))
	if err != nil {
		t.Fatal(err)
	}

	// b2_create_bucket
	bname := id + "-" + bucketName
	bucket, err := b2.CreateBucket(ctx, bname, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		// b2_delete_bucket
		if err := bucket.DeleteBucket(ctx); err != nil {
			t.Error(err)
		}
	}()

	// b2_get_upload_url
	ue, err := bucket.GetUploadURL(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// b2_upload_file
	smallFile := io.LimitReader(zReader{}, 128)
	hash := sha1.New()
	buf := &bytes.Buffer{}
	w := io.MultiWriter(hash, buf)
	if _, err := io.Copy(w, smallFile); err != nil {
		t.Error(err)
	}
	smallSHA1 := fmt.Sprintf("%x", hash.Sum(nil))
	file, err := ue.UploadFile(ctx, buf, buf.Len(), filename, "application/octet-stream", smallSHA1, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		// b2_delete_file_version
		if err := file.DeleteFileVersion(ctx); err != nil {
			t.Error(err)
		}
	}()

	// b2_download_file_by_name
	fr, err := bucket.DownloadFileByName(ctx, filename, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	lbuf := &bytes.Buffer{}
	if _, err := io.Copy(lbuf, fr); err != nil {
		t.Fatal(err)
	}
}
