package restic

import (
	"reflect"
	"testing"
)

var uniqTests = []struct {
	before, after IDs
}{
	{
		IDs{
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
			TestParseID("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"),
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
		},
		IDs{
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
			TestParseID("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"),
		},
	},
	{
		IDs{
			TestParseID("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"),
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
		},
		IDs{
			TestParseID("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"),
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
		},
	},
	{
		IDs{
			TestParseID("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"),
			TestParseID("f658198b405d7e80db5ace1980d125c8da62f636b586c46bf81dfb856a49d0c8"),
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
		},
		IDs{
			TestParseID("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"),
			TestParseID("f658198b405d7e80db5ace1980d125c8da62f636b586c46bf81dfb856a49d0c8"),
			TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
		},
	},
}

func TestUniqIDs(t *testing.T) {
	for i, test := range uniqTests {
		uniq := test.before.Uniq()
		if !reflect.DeepEqual(uniq, test.after) {
			t.Errorf("uniqIDs() test %v failed\n  wanted: %v\n  got: %v", i, test.after, uniq)
		}
	}
}
