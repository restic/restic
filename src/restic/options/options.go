package options

import (
	"reflect"
	"restic/errors"
	"strconv"
	"strings"
	"time"
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

		if v, ok := opts[key]; ok && v != value {
			return Options{}, errors.Fatalf("key %q present more than once", key)
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

// Apply sets the options on dst via reflection, using the struct tag `option`.
func (o Options) Apply(dst interface{}) error {
	v := reflect.ValueOf(dst).Elem()

	fields := make(map[string]reflect.StructField)

	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		tag := f.Tag.Get("option")

		if tag == "" {
			continue
		}

		if _, ok := fields[tag]; ok {
			panic("option tag " + tag + " is not unique in " + v.Type().Name())
		}

		fields[tag] = f
	}

	for key, value := range o {
		field, ok := fields[key]
		if !ok {
			return errors.Fatalf("option %v is not known", key)
		}

		i := field.Index[0]
		switch v.Type().Field(i).Type.Name() {
		case "string":
			v.Field(i).SetString(value)

		case "int":
			vi, err := strconv.ParseInt(value, 0, 32)
			if err != nil {
				return err
			}

			v.Field(i).SetInt(vi)

		case "Duration":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}

			v.Field(i).SetInt(int64(d))

		default:
			panic("type " + v.Type().Field(i).Type.Name() + " not handled")
		}
	}

	return nil
}
