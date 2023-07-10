package swift

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{
		S: "swift:cnt1:/",
		Cfg: Config{
			Container:   "cnt1",
			Prefix:      "",
			Connections: 5,
		},
	},
	{
		S: "swift:cnt2:/prefix",
		Cfg: Config{Container: "cnt2",
			Prefix:      "prefix",
			Connections: 5,
		},
	},
	{
		S: "swift:cnt3:/prefix/longer",
		Cfg: Config{Container: "cnt3",
			Prefix:      "prefix/longer",
			Connections: 5,
		},
	},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}

var configTestsInvalid = []string{
	"swift://hostname/container",
	"swift:////",
	"swift://",
	"swift:////prefix",
	"swift:container",
	"swift:container:",
	"swift:container/prefix",
}

func TestParseConfigInvalid(t *testing.T) {
	for i, test := range configTestsInvalid {
		_, err := ParseConfig(test)
		if err == nil {
			t.Errorf("test %d: invalid config %s did not return an error", i, test)
			continue
		}
	}
}
