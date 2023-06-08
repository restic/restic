package local

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{S: "local:/some/path", Cfg: Config{
		Path:        "/some/path",
		Connections: 2,
	}},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}
