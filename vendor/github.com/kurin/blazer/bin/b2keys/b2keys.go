// b2keys is a small utility for managing Backblaze B2 keys.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/subcommands"
	"github.com/kurin/blazer/b2"
)

const (
	apiID  = "B2_ACCOUNT_ID"
	apiKey = "B2_SECRET_KEY"
)

func main() {
	subcommands.Register(&create{}, "")
	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}

type create struct {
	d      *time.Duration
	bucket *string
	pfx    *string
}

func (c *create) Name() string     { return "create" }
func (c *create) Synopsis() string { return "create a new application key" }
func (c *create) Usage() string {
	return "b2keys create [-bucket bucket] [-duration duration] [-prefix pfx] name capability [capability ...]"
}

func (c *create) SetFlags(fs *flag.FlagSet) {
	c.d = fs.Duration("duration", 0, "the lifetime of the new key")
	c.bucket = fs.String("bucket", "", "limit the key to the given bucket")
	c.pfx = fs.String("prefix", "", "limit the key to the objects starting with prefix")
}

func (c *create) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	id := os.Getenv(apiID)
	key := os.Getenv(apiKey)
	if id == "" || key == "" {
		fmt.Fprintf(os.Stderr, "both %s and %s must be set in the environment", apiID, apiKey)
		return subcommands.ExitUsageError
	}

	args := f.Args()
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "%s\n", c.Usage())
		return subcommands.ExitUsageError
	}
	name := args[0]
	caps := args[1:]

	var opts []b2.KeyOption
	if *c.d > 0 {
		opts = append(opts, b2.Lifetime(*c.d))
	}
	if *c.pfx != "" {
		opts = append(opts, b2.Prefix(*c.pfx))
	}
	for _, c := range caps {
		opts = append(opts, b2.Capability(c))
	}

	client, err := b2.NewClient(ctx, id, key, b2.UserAgent("b2keys"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return subcommands.ExitFailure
	}

	var cr creater = client

	if *c.bucket != "" {
		bucket, err := client.Bucket(ctx, *c.bucket)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return subcommands.ExitFailure
		}
		cr = bucket
	}

	if _, err := cr.CreateKey(ctx, name, opts...); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type creater interface {
	CreateKey(context.Context, string, ...b2.KeyOption) (*b2.Key, error)
}
