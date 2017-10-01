package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/kurin/blazer/b2"
)

const (
	apiID  = "B2_ACCOUNT_ID"
	apiKey = "B2_SECRET_KEY"
)

func main() {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	ctx := context.Background()
	client, err := b2.NewClient(ctx, id, key)
	if err != nil {
		fmt.Println(err)
		return
	}
	var wg sync.WaitGroup
	for _, name := range []string{"consistobucket", "base-tests"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if err := killBucket(ctx, client, id, name); err != nil {
				fmt.Println(err)
			}
		}(name)
	}
	wg.Wait()
}

func killBucket(ctx context.Context, client *b2.Client, id, name string) error {
	bucket, err := client.NewBucket(ctx, id+"-"+name, nil)
	if b2.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer bucket.Delete(ctx)
	cur := &b2.Cursor{}
	for {
		os, c, err := bucket.ListObjects(ctx, 1000, cur)
		if err != nil && err != io.EOF {
			return err
		}
		for _, o := range os {
			o.Delete(ctx)
		}
		if err == io.EOF {
			return nil
		}
		cur = c
	}
}
