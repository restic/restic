package restic_test

import (
	"flag"
	"testing"

	"github.com/restic/restic"
)

var scanDir = flag.String("test.scandir", ".", "test/benchmark scanning a real directory (default: .)")

func TestScanner(t *testing.T) {
	sc := restic.NewScanner(nil)

	tree, err := sc.Scan(*scanDir)
	ok(t, err)

	stats := tree.Stat()

	assert(t, stats.Files > 0,
		"no files in dir %v", *scanDir)
}

func BenchmarkScanner(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := restic.NewScanner(nil).Scan(*scanDir)
		ok(b, err)
	}
}
