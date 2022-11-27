package options

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic/internal/errors"
)

// Options holds options in the form key=value.
type Options map[string]string

var opts []Help

// Register allows registering options so that they can be listed with List.
func Register(ns string, cfg interface{}) {
	opts = appendAllOptions(opts, ns, cfg)
}

// List returns a list of all registered options (using Register()).
func List() (list []Help) {
	list = make([]Help, len(opts))
	copy(list, opts)
	return list
}

// appendAllOptions appends all options in cfg to opts, sorted by namespace.
func appendAllOptions(opts []Help, ns string, cfg interface{}) []Help {
	for _, opt := range listOptions(cfg) {
		opt.Namespace = ns
		opts = append(opts, opt)
	}

	sort.Sort(helpList(opts))
	return opts
}

// listOptions returns a list of options of cfg.
func listOptions(cfg interface{}) (opts []Help) {
	// resolve indirection if cfg is a pointer
	v := reflect.Indirect(reflect.ValueOf(cfg))

	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)

		h := Help{
			Name: f.Tag.Get("option"),
			Text: f.Tag.Get("help"),
		}

		if h.Name == "" {
			continue
		}

		opts = append(opts, h)
	}

	return opts
}

// Help contains information about an option.
type Help struct {
	Namespace string
	Name      string
	Text      string
}

type helpList []Help

// Len is the number of elements in the collection.
func (h helpList) Len() int {
	return len(h)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (h helpList) Less(i, j int) bool {
	if h[i].Namespace == h[j].Namespace {
		return h[i].Name < h[j].Name
	}

	return h[i].Namespace < h[j].Namespace
}

// Swap swaps the elements with indexes i and j.
func (h helpList) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// splitKeyValue splits at the first equals (=) sign.
func splitKeyValue(s string) (key string, value string) {
	key, value, _ = strings.Cut(s, "=")
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	return key, value
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
// The namespace argument (ns) is only used for error messages.
func (o Options) Apply(ns string, dst interface{}) error {
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
			if ns != "" {
				key = ns + "." + key
			}
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

		case "uint":
			vi, err := strconv.ParseUint(value, 0, 32)
			if err != nil {
				return err
			}

			v.Field(i).SetUint(vi)

		case "bool":
			vi, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}

			v.Field(i).SetBool(vi)

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
