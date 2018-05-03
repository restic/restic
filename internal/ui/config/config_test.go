package config

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var updateGoldenFiles = flag.Bool("update", false, "update golden files in testdata/")

func readTestFile(t testing.TB, filename string) []byte {
	data, err := ioutil.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func saveGoldenFile(t testing.TB, base string, cfg Config) {
	buf, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("error marshaling result: %v", err)
	}
	buf = append(buf, '\n')

	if err = ioutil.WriteFile(filepath.Join("testdata", base+".golden"), buf, 0644); err != nil {
		t.Fatalf("unable to update golden file: %v", err)
	}
}

func loadGoldenFile(t testing.TB, base string) Config {
	buf, err := ioutil.ReadFile(filepath.Join("testdata", base+".golden"))
	if err != nil {
		t.Fatal(err)
	}

	var cfg Config
	err = json.Unmarshal(buf, &cfg)
	if err != nil {
		t.Fatal(err)
	}

	return cfg
}

func TestRead(t *testing.T) {
	entries, err := ioutil.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		filename := entry.Name()
		if filepath.Ext(filename) != ".conf" {
			continue
		}

		base := strings.TrimSuffix(filename, ".conf")
		t.Run(base, func(t *testing.T) {
			buf := readTestFile(t, filename)

			cfg, err := Parse(buf)
			if err != nil {
				t.Fatal(err)
			}

			if *updateGoldenFiles {
				saveGoldenFile(t, base, cfg)
			}

			want := loadGoldenFile(t, base)

			if !cmp.Equal(want, cfg) {
				t.Errorf("wrong config: %v", cmp.Diff(want, cfg))
			}
		})
	}
}
