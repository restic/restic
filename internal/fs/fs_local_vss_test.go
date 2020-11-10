// +build windows

package fs

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/options"
)

func matchStrings(ptrs []string, strs []string) bool {
	if len(ptrs) != len(strs) {
		return false
	}

	for i, p := range ptrs {
		matched, err := regexp.MatchString(p, strs[i])
		if err != nil {
			panic(err)
		}
		if !matched {
			return false
		}
	}

	return true
}

func matchMap(strs []string, m map[string]struct{}) bool {
	if len(strs) != len(m) {
		return false
	}

	for _, s := range strs {
		if _, ok := m[s]; !ok {
			return false
		}
	}

	return true
}

func TestVSSConfig(t *testing.T) {
	type config struct {
		excludeAllMountPoints bool
		timeout               time.Duration
	}
	setTests := []struct {
		input  options.Options
		output config
	}{
		{
			options.Options{
				"vss.timeout": "6h38m42s",
			},
			config{
				timeout: 23922000000000,
			},
		},
		{
			options.Options{
				"vss.excludeallmountpoints": "t",
			},
			config{
				excludeAllMountPoints: true,
				timeout:               120000000000,
			},
		},
		{
			options.Options{
				"vss.excludeallmountpoints": "0",
				"vss.excludevolumes":        "",
				"vss.timeout":               "120s",
			},
			config{
				timeout: 120000000000,
			},
		},
	}
	for i, test := range setTests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			cfg, err := ParseVSSConfig(test.input)
			if err != nil {
				t.Fatal(err)
			}

			errorHandler := func(item string, err error) {
				t.Fatalf("unexpected error (%v)", err)
			}
			messageHandler := func(msg string, args ...interface{}) {
				t.Fatalf("unexpected message (%s)", fmt.Sprintf(msg, args))
			}

			dst := NewLocalVss(errorHandler, messageHandler, cfg)

			if dst.excludeAllMountPoints != test.output.excludeAllMountPoints ||
				dst.excludeVolumes != nil || dst.timeout != test.output.timeout {
				t.Fatalf("wrong result, want:\n  %#v\ngot:\n  %#v", test.output, dst)
			}
		})
	}
}

func TestParseMountPoints(t *testing.T) {
	volumeMatch := regexp.MustCompile(`^\\\\\?\\Volume\{[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}\}\\$`)

	// It's not a good idea to test functions based on GetVolumeNameForVolumeMountPoint by calling
	// GetVolumeNameForVolumeMountPoint itself, but we have restricted test environment:
	// cannot manage volumes and can only be sure that the mount point C:\ exists
	sysVolume, err := GetVolumeNameForVolumeMountPoint("C:")
	if err != nil {
		t.Fatal(err)
	}
	// We don't know a valid volume GUID path for C:\, but we'll at least check its format
	if !volumeMatch.MatchString(sysVolume) {
		t.Fatalf("invalid volume GUID path: %s", sysVolume)
	}
	sysVolumeMutated := strings.ToUpper(sysVolume[:len(sysVolume)-1])
	sysVolumeMatch := strings.ToLower(sysVolume)

	type check struct {
		volume string
		result bool
	}
	setTests := []struct {
		input  options.Options
		output []string
		checks []check
		errors []string
	}{
		{
			options.Options{
				"vss.excludevolumes": `c:;c:\;` + sysVolume + `;` + sysVolumeMutated,
			},
			[]string{
				sysVolumeMatch,
			},
			[]check{
				{`c:\`, true},
				{`c:`, true},
				{sysVolume, true},
				{sysVolumeMutated, true},
			},
			[]string{},
		},
		{
			options.Options{
				"vss.excludevolumes": `z:\nonexistent;c:;c:\windows\;\\?\Volume{39b9cac2-bcdb-4d51-97c8-0d0677d607fb}\`,
			},
			[]string{
				sysVolumeMatch,
			},
			[]check{
				{`c:\windows\`, false},
				{`\\?\Volume{39b9cac2-bcdb-4d51-97c8-0d0677d607fb}\`, false},
				{`c:`, true},
				{``, false},
			},
			[]string{
				`failed to parse vss\.excludevolumes \[z:\\nonexistent\]:.*`,
				`failed to parse vss\.excludevolumes \[c:\\windows\\\]:.*`,
				`failed to parse vss\.excludevolumes \[\\\\\?\\Volume\{39b9cac2-bcdb-4d51-97c8-0d0677d607fb\}\\\]:.*`,
				`failed to get volume from mount point \[c:\\windows\\\]:.*`,
				`failed to get volume from mount point \[\\\\\?\\Volume\{39b9cac2-bcdb-4d51-97c8-0d0677d607fb\}\\\]:.*`,
				`failed to get volume from mount point \[\]:.*`,
			},
		},
	}

	for i, test := range setTests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			cfg, err := ParseVSSConfig(test.input)
			if err != nil {
				t.Fatal(err)
			}

			var log []string
			errorHandler := func(item string, err error) {
				log = append(log, strings.TrimSpace(err.Error()))
			}
			messageHandler := func(msg string, args ...interface{}) {
				t.Fatalf("unexpected message (%s)", fmt.Sprintf(msg, args))
			}

			dst := NewLocalVss(errorHandler, messageHandler, cfg)

			if !matchMap(test.output, dst.excludeVolumes) {
				t.Fatalf("wrong result, want:\n  %#v\ngot:\n  %#v",
					test.output, dst.excludeVolumes)
			}

			for _, c := range test.checks {
				if dst.isMountPointExcluded(c.volume) != c.result {
					t.Fatalf(`wrong check: isMountPointExcluded("%s") != %v`, c.volume, c.result)
				}
			}

			if !matchStrings(test.errors, log) {
				t.Fatalf("wrong log, want:\n  %#v\ngot:\n  %#v", test.errors, log)
			}
		})
	}
}
