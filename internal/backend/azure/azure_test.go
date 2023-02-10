package azure_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/azure"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newAzureTestSuite(t testing.TB) *test.Suite {
	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatalf("cannot create transport for tests: %v", err)
	}

	return &test.Suite{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			azcfg, err := azure.ParseConfig(os.Getenv("RESTIC_TEST_AZURE_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg := azcfg.(azure.Config)
			cfg.AccountName = os.Getenv("RESTIC_TEST_AZURE_ACCOUNT_NAME")
			cfg.AccountKey = options.NewSecretString(os.Getenv("RESTIC_TEST_AZURE_ACCOUNT_KEY"))
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(azure.Config)

			ctx := context.TODO()
			be, err := azure.Create(ctx, cfg, tr)
			if err != nil {
				return nil, err
			}

			_, err = be.Stat(context.TODO(), restic.Handle{Type: restic.ConfigFile})
			if err != nil && !be.IsNotExist(err) {
				return nil, err
			}

			if err == nil {
				return nil, errors.New("config already exists")
			}

			return be, nil
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(azure.Config)
			ctx := context.TODO()
			return azure.Open(ctx, cfg, tr)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(azure.Config)
			ctx := context.TODO()
			be, err := azure.Open(ctx, cfg, tr)
			if err != nil {
				return err
			}

			return be.Delete(context.TODO())
		},
	}
}

func TestBackendAzure(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/azure.TestBackendAzure")
		}
	}()

	vars := []string{
		"RESTIC_TEST_AZURE_ACCOUNT_NAME",
		"RESTIC_TEST_AZURE_ACCOUNT_KEY",
		"RESTIC_TEST_AZURE_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}

	t.Logf("run tests")
	newAzureTestSuite(t).RunTests(t)
}

func BenchmarkBackendAzure(t *testing.B) {
	vars := []string{
		"RESTIC_TEST_AZURE_ACCOUNT_NAME",
		"RESTIC_TEST_AZURE_ACCOUNT_KEY",
		"RESTIC_TEST_AZURE_REPOSITORY",
	}

	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("environment variable %v not set", v)
			return
		}
	}

	t.Logf("run tests")
	newAzureTestSuite(t).RunBenchmarks(t)
}

func TestUploadLargeFile(t *testing.T) {
	if os.Getenv("RESTIC_AZURE_TEST_LARGE_UPLOAD") == "" {
		t.Skip("set RESTIC_AZURE_TEST_LARGE_UPLOAD=1 to test large uploads")
		return
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	if os.Getenv("RESTIC_TEST_AZURE_REPOSITORY") == "" {
		t.Skipf("environment variables not available")
		return
	}

	azcfg, err := azure.ParseConfig(os.Getenv("RESTIC_TEST_AZURE_REPOSITORY"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := azcfg.(azure.Config)
	cfg.AccountName = os.Getenv("RESTIC_TEST_AZURE_ACCOUNT_NAME")
	cfg.AccountKey = options.NewSecretString(os.Getenv("RESTIC_TEST_AZURE_ACCOUNT_KEY"))
	cfg.Prefix = fmt.Sprintf("test-upload-large-%d", time.Now().UnixNano())

	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatal(err)
	}

	be, err := azure.Create(ctx, cfg, tr)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err := be.Delete(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}()

	data := rtest.Random(23, 300*1024*1024)
	id := restic.Hash(data)
	h := restic.Handle{Name: id.String(), Type: restic.PackFile}

	t.Logf("hash of %d bytes: %v", len(data), id)

	err = be.Save(ctx, h, restic.NewByteReader(data, be.Hasher()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := be.Remove(ctx, h)
		if err != nil {
			t.Fatal(err)
		}
	}()

	var tests = []struct {
		offset, length int
	}{
		{0, len(data)},
		{23, 1024},
		{23 + 100*1024, 500},
		{888 + 200*1024, 89999},
		{888 + 100*1024*1024, 120 * 1024 * 1024},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			want := data[test.offset : test.offset+test.length]

			buf := make([]byte, test.length)
			err = be.Load(ctx, h, test.length, int64(test.offset), func(rd io.Reader) error {
				_, err = io.ReadFull(rd, buf)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf, want) {
				t.Fatalf("wrong bytes returned")
			}
		})
	}
}
