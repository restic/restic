package debug_test

import (
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"testing"
)

func BenchmarkLogStatic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		debug.Log("Static string")
	}
}

func BenchmarkLogIDStr(b *testing.B) {
	id := restic.NewRandomID()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		debug.Log("id: %v", id)
	}
}

func BenchmarkLogIDString(b *testing.B) {
	id := restic.NewRandomID()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		debug.Log("id: %s", id)
	}
}
