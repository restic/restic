// +build windows

package fs

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/restic/restic/internal/options"
)

func matchStrings(ptrs []string, strs []string) bool {
	if len(ptrs) != len(strs) {
		return false
	}

	for i, p := range ptrs {
		if p == "" {
			return false
		}
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
		provider              string
	}
	setTests := []struct {
		input  options.Options
		output config
	}{
		{
			options.Options{
				"vss.timeout":  "6h38m42s",
				"vss.provider": "Ms",
			},
			config{
				timeout:  23922000000000,
				provider: "Ms",
			},
		},
		{
			options.Options{
				"vss.exclude-all-mount-points": "t",
				"vss.provider":                 "{b5946137-7b9f-4925-af80-51abd60b20d5}",
			},
			config{
				excludeAllMountPoints: true,
				timeout:               120000000000,
				provider:              "{b5946137-7b9f-4925-af80-51abd60b20d5}",
			},
		},
		{
			options.Options{
				"vss.exclude-all-mount-points": "0",
				"vss.exclude-volumes":          "",
				"vss.timeout":                  "120s",
				"vss.provider":                 "Microsoft Software Shadow Copy provider 1.0",
			},
			config{
				timeout:  120000000000,
				provider: "Microsoft Software Shadow Copy provider 1.0",
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
				dst.excludeVolumes != nil || dst.timeout != test.output.timeout ||
				dst.provider != test.output.provider {
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
	// We don't know a valid volume GUID path for c:\, but we'll at least check its format
	if !volumeMatch.MatchString(sysVolume) {
		t.Fatalf("invalid volume GUID path: %s", sysVolume)
	}
	// Changing the case and removing trailing backslash allows tests
	// the equality of different ways of writing a volume name
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
				"vss.exclude-volumes": `c:;c:\;` + sysVolume + `;` + sysVolumeMutated,
			},
			[]string{
				sysVolumeMatch,
			},
			[]check{
				{`c:\`, false},
				{`c:`, false},
				{sysVolume, false},
				{sysVolumeMutated, false},
			},
			[]string{},
		},
		{
			options.Options{
				"vss.exclude-volumes": `z:\nonexistent;c:;c:\windows\;\\?\Volume{39b9cac2-bcdb-4d51-97c8-0d0677d607fb}\`,
			},
			[]string{
				sysVolumeMatch,
			},
			[]check{
				{`c:\windows\`, true},
				{`\\?\Volume{39b9cac2-bcdb-4d51-97c8-0d0677d607fb}\`, true},
				{`c:`, false},
				{``, true},
			},
			[]string{
				`failed to parse vss\.exclude-volumes \[z:\\nonexistent\]:.*`,
				`failed to parse vss\.exclude-volumes \[c:\\windows\\\]:.*`,
				`failed to parse vss\.exclude-volumes \[\\\\\?\\Volume\{39b9cac2-bcdb-4d51-97c8-0d0677d607fb\}\\\]:.*`,
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
				if dst.isMountPointIncluded(c.volume) != c.result {
					t.Fatalf(`wrong check: isMountPointIncluded("%s") != %v`, c.volume, c.result)
				}
			}

			if !matchStrings(test.errors, log) {
				t.Fatalf("wrong log, want:\n  %#v\ngot:\n  %#v", test.errors, log)
			}
		})
	}
}

func TestParseProvider(t *testing.T) {
	msProvider := ole.NewGUID("{b5946137-7b9f-4925-af80-51abd60b20d5}")
	setTests := []struct {
		provider string
		id       *ole.GUID
		result   string
	}{
		{
			"",
			ole.IID_NULL,
			"",
		},
		{
			"mS",
			msProvider,
			"",
		},
		{
			"{B5946137-7b9f-4925-Af80-51abD60b20d5}",
			msProvider,
			"",
		},
		{
			"Microsoft Software Shadow Copy provider 1.0",
			msProvider,
			"",
		},
		{
			"{04560982-3d7d-4bbc-84f7-0712f833a28f}",
			nil,
			`invalid VSS provider "{04560982-3d7d-4bbc-84f7-0712f833a28f}"`,
		},
		{
			"non-existent provider",
			nil,
			`invalid VSS provider "non-existent provider"`,
		},
	}

	_ = ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)

	for i, test := range setTests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			id, err := getProviderID(test.provider)

			if err != nil && id != nil {
				t.Fatalf("err!=nil but id=%v", id)
			}

			if test.result != "" || err != nil {
				var result string
				if err != nil {
					result = err.Error()
				}
				if test.result != result || test.result == "" {
					t.Fatalf("wrong result, want:\n  %#v\ngot:\n  %#v", test.result, result)
				}
			} else if !ole.IsEqualGUID(id, test.id) {
				t.Fatalf("wrong id, want:\n  %s\ngot:\n  %s", test.id.String(), id.String())
			}
		})
	}
}
