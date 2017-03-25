package options

import (
	"restic/errors"
	"strings"
)

// Options holds options in the form key=value.
type Options map[string]string

// splitKeyValue splits at the first equals (=) sign.
func splitKeyValue(s string) (key string, value string) {
	data := strings.SplitN(s, "=", 2)
	key = strings.ToLower(strings.TrimSpace(data[0]))
	if len(data) == 1 {
		// no equals sign is treated as the empty value
		return key, ""
	}

	return key, strings.TrimSpace(data[1])
}

// Parse takes a slice of key=value pairs and returns an Options type.
// The key may include namespaces, separated by dots. Example: "foo.bar=value".
// Keys are converted to lower-case.
func Parse(in []string) (Options, error) {
	opts := make(Options, len(in))

	for _, opt := range in {
		key, value := splitKeyValue(opt)

		if key == "" {
			return Options{}, errors.Fatalf("empty key is not a valid option")
		}
		opts[key] = value
	}

	return opts, nil
}

// Extract returns an Options type with all keys in namespace ns, which is
// also stripped from the keys. ns must end with a dot.
func (o Options) Extract(ns string) Options {
	l := len(ns)
	if ns[l-1] != '.' {
		ns += "."
		l++
	}

	opts := make(Options)

	for k, v := range o {
		if !strings.HasPrefix(k, ns) {
			continue
		}

		opts[k[l:]] = v
	}

	return opts
}
