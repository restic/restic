package location_test

import (
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/test"
)

type testConfig struct {
	loc string
}

func testFactory() location.Factory {
	return location.NewHTTPBackendFactory[testConfig, backend.Backend](
		"local",
		func(s string) (*testConfig, error) {
			return &testConfig{loc: s}, nil
		}, nil, nil, nil,
	)
}

func TestParse(t *testing.T) {
	registry := location.NewRegistry()
	registry.Register(testFactory())

	path := "local:example"
	u, err := location.Parse(registry, path)
	test.OK(t, err)
	test.Equals(t, "local", u.Scheme)
	test.Equals(t, &testConfig{loc: path}, u.Config)
}

func TestParseFallback(t *testing.T) {
	fallbackTests := []string{
		"dir1/dir2",
		"/dir1/dir2",
		"/dir1:foobar/dir2",
		`\dir1\foobar\dir2`,
		`c:\dir1\foobar\dir2`,
		`C:\Users\appveyor\AppData\Local\Temp\1\restic-test-879453535\repo`,
		`c:/dir1/foobar/dir2`,
	}

	registry := location.NewRegistry()
	registry.Register(testFactory())

	for _, path := range fallbackTests {
		t.Run(path, func(t *testing.T) {
			u, err := location.Parse(registry, path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			test.Equals(t, "local", u.Scheme)
			test.Equals(t, "local:"+path, u.Config.(*testConfig).loc)
		})
	}
}

func TestInvalidScheme(t *testing.T) {
	registry := location.NewRegistry()
	var invalidSchemes = []string{
		"foobar:xxx",
		"foobar:/dir/dir2",
	}

	for _, s := range invalidSchemes {
		t.Run(s, func(t *testing.T) {
			_, err := location.Parse(registry, s)
			if err == nil {
				t.Fatalf("error for invalid location %q not found", s)
			}
		})
	}
}
