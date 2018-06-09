Blazer
====

[![GoDoc](https://godoc.org/github.com/kurin/blazer/b2?status.svg)](https://godoc.org/github.com/kurin/blazer/b2)
[![Build Status](https://travis-ci.org/kurin/blazer.svg)](https://travis-ci.org/kurin/blazer)

Blazer is a Golang client library for Backblaze's B2 object storage service.
It is designed for simple integration with existing applications that may
already be using S3 and Google Cloud Storage, by exporting only a few standard
Go types.

It implements and satisfies the [B2 integration
checklist](https://www.backblaze.com/b2/docs/integration_checklist.html),
automatically handling error recovery, reauthentication, and other low-level
aspects, making it suitable to upload very large files, or over multi-day time
scales.

```go
import "github.com/kurin/blazer/b2"
```

## Examples

### Copy a file into B2

```go
func copyFile(ctx context.Context, bucket *b2.Bucket, src, dst string) error {
	f, err := file.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	obj := bucket.Object(dst)
	w := obj.NewWriter(ctx)
	if _, err := io.Copy(w, f); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
```

If the file is less than 100MB, Blazer will simply buffer the file and use the
`b2_upload_file` API to send the file to Backblaze.  If the file is greater
than 100MB, Blazer will use B2's large file support to upload the file in 100MB
chunks.

### Copy a file into B2, with multiple concurrent uploads

Uploading a large file with multiple HTTP connections is simple:

```go
func copyFile(ctx context.Context, bucket *b2.Bucket, writers int, src, dst string) error {
	f, err := file.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bucket.Object(dst).NewWriter(ctx)
	w.ConcurrentUploads = writers
	if _, err := io.Copy(w, f); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
```

This will automatically split the file into `writers` chunks of 100MB uploads.
Note that 100MB is the smallest chunk size that B2 supports.

### Download a file from B2

Downloading is as simple as uploading:

```go
func downloadFile(ctx context.Context, bucket *b2.Bucket, downloads int, src, dst string) error {
	r := bucket.Object(src).NewReader(ctx)
	defer r.Close()

	f, err := file.Create(dst)
	if err != nil {
		return err
	}
	r.ConcurrentDownloads = downloads
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
```

### List all objects in a bucket

```go
func printObjects(ctx context.Context, bucket *b2.Bucket) error {
	iterator := bucket.List(ctx)
	for iterator.Next() {
		fmt.Println(itrator.Object())
	}
	return iterator.Err()
}
```

### Grant temporary auth to a file

Say you have a number of files in a private bucket, and you want to allow other
people to download some files.  This is possible to do by issuing a temporary
authorization token for the prefix of the files you want to share.

```go
token, err := bucket.AuthToken(ctx, "photos", time.Hour)
```

If successful, `token` is then an authorization token valid for one hour, which
can be set in HTTP GET requests.

The hostname to use when downloading files via HTTP is account-specific and can
be found via the BaseURL method:

```go
base := bucket.BaseURL()
```

---

This is not an official Google product.
