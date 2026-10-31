package drime

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

func drimeConfig(p, root string) Config {
	cfg := NewConfig()
	cfg.Path = p
	cfg.RootFolderID = root
	return cfg
}

var configTests = []test.ConfigTestData[Config]{
	{S: "drime:backups", Cfg: drimeConfig("backups", "")},
	{S: "drime:/backups/restic/", Cfg: drimeConfig("backups/restic", "")},
	{S: "drime:a//b/../c", Cfg: drimeConfig("a/c", "")},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}

func TestInvalidConfig(t *testing.T) {
	for _, s := range []string{"drime", "drime:", "drime:/", "drime:..", "other:foo"} {
		t.Run(s, func(t *testing.T) {
			if _, err := ParseConfig(s); err == nil {
				t.Fatalf("expected error for %q", s)
			}
		})
	}
}
