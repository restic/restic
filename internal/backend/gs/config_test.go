package gs

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{S: "gs:bucketname:/", Cfg: Config{
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
		Region:      "us",
	}},
	{S: "gs:bucketname:/prefix/directory", Cfg: Config{
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
		Region:      "us",
	}},
	{S: "gs:bucketname:/prefix/directory/", Cfg: Config{
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
		Region:      "us",
	}},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}
