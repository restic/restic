//go:build !linux

package tracing

import (
	"testing"
)

func TestCollectAncestry(t *testing.T) {
	chain := collectAncestry()
	if chain != nil {
		t.Errorf("expected nil ancestry on non-linux, got %v", chain)
	}
}
