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
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"golang.org/x/net/context"
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
	b2, err := AuthorizeAccount(ctx, id, key)
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
		"one_BILLION":  "1e9",
		"two_TRILLION": "2eSomething, I guess 2e12",
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
