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
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newAzureTestSuite() *test.Suite[azure.Config] {
	return &test.Suite[azure.Config]{
		// do not use excessive data
		MinimalData: true,

		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*azure.Config, error) {
			cfg, err := azure.ParseConfig(os.Getenv("RESTIC_TEST_AZURE_REPOSITORY"))
			if err != nil {
				return nil, err
			}

			cfg.ApplyEnvironment("RESTIC_TEST_")
			cfg.Prefix = fmt.Sprintf("test-%d", time.Now().UnixNano())
			return cfg, nil
		},

		Factory: azure.NewFactory(),
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
	newAzureTestSuite().RunTests(t)
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
	newAzureTestSuite().RunBenchmarks(t)
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

	cfg, err := azure.ParseConfig(os.Getenv("RESTIC_TEST_AZURE_REPOSITORY"))
	if err != nil {
		t.Fatal(err)
	}

	cfg.AccountName = os.Getenv("RESTIC_TEST_AZURE_ACCOUNT_NAME")
	cfg.AccountKey = options.NewSecretString(os.Getenv("RESTIC_TEST_AZURE_ACCOUNT_KEY"))
	cfg.Prefix = fmt.Sprintf("test-upload-large-%d", time.Now().UnixNano())

	tr, err := backend.Transport(backend.TransportOptions{})
	if err != nil {
		t.Fatal(err)
	}

	be, err := azure.Create(ctx, *cfg, tr)
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
	h := backend.Handle{Name: id.String(), Type: backend.PackFile}

	t.Logf("hash of %d bytes: %v", len(data), id)

	err = be.Save(ctx, h, backend.NewByteReader(data, be.Hasher()))
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
