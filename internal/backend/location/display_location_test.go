package location_test

import (
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/test"
)

func TestStripPassword(t *testing.T) {
	registry := location.NewRegistry()
	registry.Register(
		location.NewHTTPBackendFactory[any, backend.Backend]("test", nil,
			func(s string) string {
				return "cleaned"
			}, nil, nil,
		),
	)

	t.Run("valid", func(t *testing.T) {
		clean := location.StripPassword(registry, "test:secret")
		test.Equals(t, "cleaned", clean)
	})
	t.Run("unknown", func(t *testing.T) {
		clean := location.StripPassword(registry, "invalid:secret")
		test.Equals(t, "invalid:secret", clean)
	})
}
