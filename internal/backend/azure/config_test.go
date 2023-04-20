package azure

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{"azure:container-name:/", Config{
		Container:   "container-name",
		Prefix:      "",
		Connections: 5,
	}},
	{"azure:container-name:/prefix/directory", Config{
		Container:   "container-name",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{"azure:container-name:/prefix/directory/", Config{
		Container:   "container-name",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}
