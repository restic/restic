package config

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"
)

var updateGoldenFiles = flag.Bool("update", false, "update golden files in testdata/")

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

func TestConfigLoad(t *testing.T) {
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
			cfg, err := Load(filepath.Join("testdata", filename))
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

func TestConfigApplyFlags(t *testing.T) {
	var tests = []struct {
		filename   string
		applyFlags func(cfg *Config) error
		want       Config
	}{
		{
			filename: "backup.conf",
			applyFlags: func(cfg *Config) error {
				args := []string{"--exclude", "foo/*.go"}

				s := pflag.NewFlagSet("", pflag.ContinueOnError)
				s.StringArrayP("exclude", "e", nil, "exclude files")

				err := s.Parse(args)
				if err != nil {
					return err
				}

				return ApplyFlags(&cfg.Backup, s)
			},
			want: Config{
				Backup: Backup{
					Target:   []string{"foo", "/home/user"},
					Excludes: []string{"foo/*.go"},
				},
				Backends: map[string]Backend{},
			},
		},
		{
			filename: "backup.conf",
			applyFlags: func(cfg *Config) error {
				args := []string{"--repo", "sftp:user@server:/srv/backup/repo"}

				s := pflag.NewFlagSet("", pflag.ContinueOnError)
				s.StringP("repo", "r", "", "repository to backup to or restore from")

				err := s.Parse(args)
				if err != nil {
					return err
				}

				return ApplyFlags(cfg, s)
			},
			want: Config{
				Backup: Backup{
					Target: []string{"foo", "/home/user"},
				},
				Repo:     "sftp:user@server:/srv/backup/repo",
				Backends: map[string]Backend{},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			cfg, err := Load(filepath.Join("testdata", test.filename))
			if err != nil {
				t.Fatal(err)
			}

			err = test.applyFlags(&cfg)
			if err != nil {
				t.Fatal(err)
			}

			if !cmp.Equal(test.want, cfg) {
				t.Error(cmp.Diff(test.want, cfg))
			}
		})
	}
}

func TestConfigApplyEnv(t *testing.T) {
	var tests = []struct {
		filename string
		env      []string
		want     Config
	}{
		{
			filename: "backup.conf",
			env: []string{
				"RESTIC_REPOSITORY=/tmp/repo",
				"RESTIC_PASSWORD=foobar",
				"RESTIC_PASSWORD_FILE=/root/secret.txt",
			},
			want: Config{
				Password:     "foobar",
				PasswordFile: "/root/secret.txt",
				Repo:         "/tmp/repo",
				Backup: Backup{
					Target: []string{"foo", "/home/user"},
				},
				Backends: map[string]Backend{},
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			cfg, err := Load(filepath.Join("testdata", test.filename))
			if err != nil {
				t.Fatal(err)
			}

			err = ApplyEnv(&cfg, test.env)
			if err != nil {
				t.Fatal(err)
			}

			if !cmp.Equal(test.want, cfg) {
				t.Error(cmp.Diff(test.want, cfg))
			}
		})
	}
}
