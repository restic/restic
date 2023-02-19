package restic

import (
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestIDsString(t *testing.T) {
	ids := IDs{
		TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
		TestParseID("1285b30394f3b74693cc29a758d9624996ae643157776fce8154aabd2f01515f"),
		TestParseID("7bb086db0d06285d831485da8031281e28336a56baa313539eaea1c73a2a1a40"),
	}

	rtest.Equals(t, "[7bb086db 1285b303 7bb086db]", ids.String())
}
