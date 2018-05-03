package config

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/spf13/pflag"
)

// Config contains configuration items read from a file.
type Config struct {
	Repo         string `config:"repo"          flag:"repo"          env:"RESTIC_REPOSITORY"`
	Password     string `config:"password"                           env:"RESTIC_PASSWORD"`
	PasswordFile string `config:"password_file" flag:"password-file" env:"RESTIC_PASSWORD_FILE"`

	Backends map[string]Backend `config:"backend"`
	Backup   Backup             `config:"backup"`
}

// Backend is a configured backend to store a repository.
type Backend struct {
	Backend string
	Path    string
}

// Backup sets the options for the "backup" command.
type Backup struct {
	Target   []string `config:"target"`
	Excludes []string `config:"exclude" flag:"exclude"`
}

// listTags returns the all the top-level tags with the name tagname of obj.
func listTags(obj interface{}, tagname string) map[string]struct{} {
	list := make(map[string]struct{})

	// resolve indirection if obj is a pointer
	v := reflect.Indirect(reflect.ValueOf(obj))

	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)

		val := f.Tag.Get(tagname)
		list[val] = struct{}{}
	}

	return list
}

func validateObjects(list *ast.ObjectList, validNames map[string]struct{}) error {
	for _, item := range list.Items {
		ident := item.Keys[0].Token.Value().(string)
		if _, ok := validNames[ident]; !ok {
			return errors.Errorf("unknown option %q found at line %v, column %v",
				ident, item.Pos().Line, item.Pos().Column)
		}
	}

	return nil
}

// Parse parses a config file from buf.
func Parse(buf []byte) (cfg Config, err error) {
	parsed, err := hcl.ParseBytes(buf)
	if err != nil {
		return Config{}, err
	}

	err = hcl.DecodeObject(&cfg, parsed)
	if err != nil {
		return Config{}, err
	}

	// check for additional unknown items
	root := parsed.Node.(*ast.ObjectList)

	checks := map[string]map[string]struct{}{
		"":       listTags(cfg, "config"),
		"backup": listTags(Backup{}, "config"),
	}

	for name, valid := range checks {
		list := root
		if name != "" {
			if len(root.Filter(name).Items) == 0 {
				continue
			}

			val := root.Filter(name).Items[0].Val
			obj, ok := val.(*ast.ObjectType)

			if !ok {
				return Config{}, errors.Errorf("error in line %v, column %v: %q must be an object", val.Pos().Line, val.Pos().Column, name)
			}
			list = obj.List
		}

		err = validateObjects(list, valid)
		if err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

// Load loads a config from a file.
func Load(filename string) (Config, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return Config{}, err
	}

	return Parse(buf)
}

func getFieldsForTag(tagname string, target interface{}) map[string]reflect.Value {
	v := reflect.ValueOf(target).Elem()
	// resolve indirection
	vi := reflect.Indirect(reflect.ValueOf(target))

	attr := make(map[string]reflect.Value)
	for i := 0; i < vi.NumField(); i++ {
		typeField := vi.Type().Field(i)
		tag := typeField.Tag.Get(tagname)
		if tag == "" {
			continue
		}

		field := v.FieldByName(typeField.Name)

		if !field.CanSet() {
			continue
		}

		attr[tag] = field
	}

	return attr
}

// ApplyFlags takes the values from the flag set and applies them to cfg.
func ApplyFlags(cfg interface{}, fset *pflag.FlagSet) error {
	if reflect.TypeOf(cfg).Kind() != reflect.Ptr {
		panic("target config is not a pointer")
	}

	attr := getFieldsForTag("flag", cfg)

	var visitError error
	fset.VisitAll(func(flag *pflag.Flag) {
		if visitError != nil {
			return
		}

		field, ok := attr[flag.Name]
		if !ok {
			return
		}

		if !flag.Changed {
			return
		}

		debug.Log("apply flag %v, to field %v\n", flag.Name, field.Type().Name())

		switch flag.Value.Type() {
		case "count":
			v, err := fset.GetCount(flag.Name)
			if err != nil {
				visitError = err
				return
			}
			field.SetUint(uint64(v))
		case "bool":
			v, err := fset.GetBool(flag.Name)
			if err != nil {
				visitError = err
				return
			}
			field.SetBool(v)
		case "string":
			v, err := fset.GetString(flag.Name)
			if err != nil {
				visitError = err
				return
			}
			field.SetString(v)
		case "stringArray":
			v, err := fset.GetStringArray(flag.Name)
			if err != nil {
				visitError = err
				return
			}
			field.SetSlice(v)
		default:
			visitError = errors.Errorf("flag %v has unknown type %v", flag.Name, flag.Value.Type())
			return
		}
	})

	return visitError
}

// ApplyEnv takes the list of environment variables and applies them to the
// config.
func ApplyEnv(cfg interface{}, env []string) error {
	attr := getFieldsForTag("env", cfg)

	for _, s := range env {
		data := strings.SplitN(s, "=", 2)
		if len(data) != 2 {
			continue
		}

		name, value := data[0], data[1]
		field, ok := attr[name]
		if !ok {
			continue
		}

		if field.Kind() != reflect.String {
			panic(fmt.Sprintf("unsupported field type %v", field.Kind()))
		}

		debug.Log("apply env %v (%q) to %v\n", name, value, field.Type().Name())
		field.SetString(value)
	}

	return nil
}
