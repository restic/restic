// Copyright 2017, Google
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

// This is a simple program that will copy named files into or out of B2.
//
// To copy a file into B2:
//
//     B2_ACCOUNT_ID=foo B2_ACCOUNT_KEY=bar simple /path/to/file b2://bucket/path/to/dst
//
// To copy a file out:
//
//     B2_ACCOUNT_ID=foo B2_ACCOUNT_KEY=bar simple b2://bucket/path/to/file /path/to/dst
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/kurin/blazer/b2"
)

func main() {
	flag.Parse()
	b2id := os.Getenv("B2_ACCOUNT_ID")
	b2key := os.Getenv("B2_ACCOUNT_KEY")

	args := flag.Args()
	if len(args) != 2 {
		fmt.Printf("Usage:\n\nsimple [src] [dst]\n")
		return
	}
	src, dst := args[0], args[1]

	ctx := context.Background()
	c, err := b2.NewClient(ctx, b2id, b2key)
	if err != nil {
		fmt.Println(err)
		return
	}

	var r io.ReadCloser
	var w io.WriteCloser

	if strings.HasPrefix(src, "b2://") {
		reader, err := b2Reader(ctx, c, src)
		if err != nil {
			fmt.Println(err)
			return
		}
		r = reader
	} else {
		f, err := os.Open(src)
		if err != nil {
			fmt.Println(err)
			return
		}
		r = f
	}
	// Readers do not need their errors checked on close.  (Also it's a little
	// silly to defer this in main(), but.)
	defer r.Close()

	if strings.HasPrefix(dst, "b2://") {
		writer, err := b2Writer(ctx, c, dst)
		if err != nil {
			fmt.Println(err)
			return
		}
		w = writer
	} else {
		f, err := os.Create(dst)
		if err != nil {
			fmt.Println(err)
			return
		}
		w = f
	}

	// Copy and check error.
	if _, err := io.Copy(w, r); err != nil {
		fmt.Println(err)
		return
	}

	// It is very important to check the error of the writer.
	if err := w.Close(); err != nil {
		fmt.Println(err)
	}
}

func b2Reader(ctx context.Context, c *b2.Client, path string) (io.ReadCloser, error) {
	o, err := b2Obj(ctx, c, path)
	if err != nil {
		return nil, err
	}
	return o.NewReader(ctx), nil
}

func b2Writer(ctx context.Context, c *b2.Client, path string) (io.WriteCloser, error) {
	o, err := b2Obj(ctx, c, path)
	if err != nil {
		return nil, err
	}
	return o.NewWriter(ctx), nil
}

func b2Obj(ctx context.Context, c *b2.Client, path string) (*b2.Object, error) {
	uri, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	bucket, err := c.Bucket(ctx, uri.Host)
	if err != nil {
		return nil, err
	}
	// B2 paths must not begin with /, so trim it here.
	return bucket.Object(strings.TrimPrefix(uri.Path, "/")), nil
}
