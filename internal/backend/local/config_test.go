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
	{S: "local:dir1/dir2", Cfg: Config{
		Path:        "dir1/dir2",
		Connections: 2,
	}},
	{S: "local:../dir1/dir2", Cfg: Config{
		Path:        "../dir1/dir2",
		Connections: 2,
	}},
	{S: "local:/dir1:foobar/dir2", Cfg: Config{
		Path:        "/dir1:foobar/dir2",
		Connections: 2,
	}},
	{S: `local:\dir1\foobar\dir2`, Cfg: Config{
		Path:        `\dir1\foobar\dir2`,
		Connections: 2,
	}},
	{S: `local:c:\dir1\foobar\dir2`, Cfg: Config{
		Path:        `c:\dir1\foobar\dir2`,
		Connections: 2,
	}},
	{S: `local:C:\Users\appveyor\AppData\Local\Temp\1\restic-test-879453535\repo`, Cfg: Config{
		Path:        `C:\Users\appveyor\AppData\Local\Temp\1\restic-test-879453535\repo`,
		Connections: 2,
	}},
	{S: `local:c:/dir1/foobar/dir2`, Cfg: Config{
		Path:        `c:/dir1/foobar/dir2`,
		Connections: 2,
	}},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}
