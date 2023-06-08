package azure

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{S: "azure:container-name:/", Cfg: Config{
		Container:   "container-name",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "azure:container-name:/prefix/directory", Cfg: Config{
		Container:   "container-name",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{S: "azure:container-name:/prefix/directory/", Cfg: Config{
		Container:   "container-name",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}
