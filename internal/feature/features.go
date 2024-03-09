package feature

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type state string
type FlagName string

const (
	// Alpha features are disabled by default. They do not guarantee any backwards compatibility and may change in arbitrary ways between restic versions.
	Alpha state = "alpha"
	// Beta features are enabled by default. They may still change, but incompatible changes should be avoided.
	Beta state = "beta"
	// Stable features are always enabled
	Stable state = "stable"
	// Deprecated features are always disabled
	Deprecated state = "deprecated"
)

type FlagDesc struct {
	Type        state
	Description string
}

type FlagSet struct {
	flags   map[FlagName]*FlagDesc
	enabled map[FlagName]bool
}

func New() *FlagSet {
	return &FlagSet{}
}

func getDefault(phase state) bool {
	switch phase {
	case Alpha, Deprecated:
		return false
	case Beta, Stable:
		return true
	default:
		panic("unknown feature phase")
	}
}

func (f *FlagSet) SetFlags(flags map[FlagName]FlagDesc) {
	f.flags = map[FlagName]*FlagDesc{}
	f.enabled = map[FlagName]bool{}

	for name, flag := range flags {
		fcopy := flag
		f.flags[name] = &fcopy
		f.enabled[name] = getDefault(fcopy.Type)
	}
}

func (f *FlagSet) Apply(flags string, logWarning func(string)) error {
	if flags == "" {
		return nil
	}

	selection := make(map[string]bool)

	for _, flag := range strings.Split(flags, ",") {
		parts := strings.SplitN(flag, "=", 2)

		name := parts[0]
		value := "true"
		if len(parts) == 2 {
			value = parts[1]
		}

		isEnabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("failed to parse value %q for feature flag %v: %w", value, name, err)
		}

		selection[name] = isEnabled
	}

	for name, value := range selection {
		fname := FlagName(name)
		flag := f.flags[fname]
		if flag == nil {
			return fmt.Errorf("unknown feature flag %q", name)
		}

		switch flag.Type {
		case Alpha, Beta:
			f.enabled[fname] = value
		case Stable:
			logWarning(fmt.Sprintf("feature flag %q is always enabled and will be removed in a future release", fname))
		case Deprecated:
			logWarning(fmt.Sprintf("feature flag %q is always disabled and will be removed in a future release", fname))
		default:
			panic("unknown feature phase")
		}
	}

	return nil
}

func (f *FlagSet) Enabled(name FlagName) bool {
	isEnabled, ok := f.enabled[name]
	if !ok {
		panic(fmt.Sprintf("unknown feature flag %v", name))
	}

	return isEnabled
}

// Help contains information about a feature.
type Help struct {
	Name        string
	Type        string
	Default     bool
	Description string
}

func (f *FlagSet) List() []Help {
	var help []Help

	for name, flag := range f.flags {
		help = append(help, Help{
			Name:        string(name),
			Type:        string(flag.Type),
			Default:     getDefault(flag.Type),
			Description: flag.Description,
		})
	}

	sort.Slice(help, func(i, j int) bool {
		return strings.Compare(help[i].Name, help[j].Name) < 0
	})

	return help
}
