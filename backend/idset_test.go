package backend_test

import (
	"testing"

	"github.com/restic/restic/backend"
)

var idsetTests = []struct {
	id   backend.ID
	seen bool
}{
	{str2id("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"), false},
	{str2id("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"), false},
	{str2id("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"), true},
	{str2id("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"), true},
	{str2id("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"), true},
	{str2id("f658198b405d7e80db5ace1980d125c8da62f636b586c46bf81dfb856a49d0c8"), false},
	{str2id("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"), true},
	{str2id("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"), true},
	{str2id("f658198b405d7e80db5ace1980d125c8da62f636b586c46bf81dfb856a49d0c8"), true},
	{str2id("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"), true},
}

func TestIDSet(t *testing.T) {
	set := backend.NewIDSet()
	for i, test := range idsetTests {
		seen := set.Has(test.id)
		if seen != test.seen {
			t.Errorf("IDSet test %v failed: wanted %v, got %v", i, test.seen, seen)
		}
		set.Insert(test.id)
	}
}
