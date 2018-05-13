package config

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/hashicorp/hcl/hcl/token"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/spf13/pflag"
)

// Config contains configuration items read from a file.
type Config struct {
	Repo         string `hcl:"repo"          flag:"repo"          env:"RESTIC_REPOSITORY"`
	Password     string `hcl:"password"                           env:"RESTIC_PASSWORD"`
	PasswordFile string `hcl:"password_file" flag:"password-file" env:"RESTIC_PASSWORD_FILE"`

	Backends map[string]interface{}
	Backup   Backup `hcl:"backup"`
}

// Backend configures a backend.
type Backend struct {
	Type string `hcl:"type"`
}

// BackendLocal is a backend in a local directory.
type BackendLocal struct {
	Type string `hcl:"type"`
	Path string `hcl:"path"`
}

// BackendSFTP is a backend stored on a server via sftp.
type BackendSFTP struct {
	Type string `hcl:"type"`
	User string `hcl:"user"`
	Host string `hcl:"host"`
}

// Backup sets the options for the "backup" command.
type Backup struct {
	Target   []string `hcl:"target"`
	Excludes []string `hcl:"exclude" flag:"exclude"`
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

	root := parsed.Node.(*ast.ObjectList)

	// load all 'backend' sections
	cfg.Backends, err = parseBackends(root)
	if err != nil {
		return Config{}, err
	}

	// check for additional unknown items
	rootTags := listTags(cfg, "hcl")
	rootTags["backend"] = struct{}{}

	checks := map[string]map[string]struct{}{
		"":       rootTags,
		"backup": listTags(Backup{}, "hcl"),
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

// parseBackends parses the backend configuration sections.
func parseBackends(root *ast.ObjectList) (map[string]interface{}, error) {
	backends := make(map[string]interface{})

	// find top-level backend objects
	for _, item := range root.Items {
		// is not an object block
		if len(item.Keys) == 0 {
			continue
		}

		// does not start with an an identifier
		if item.Keys[0].Token.Type != token.IDENT {
			continue
		}

		// something other than a backend section
		if s, ok := item.Keys[0].Token.Value().(string); !ok || s != "backend" {
			continue
		}

		// missing name
		if len(item.Keys) != 2 {
			return nil, errors.Errorf("backend has no name at line %v, column %v",
				item.Pos().Line, item.Pos().Column)
		}

		// check that the name is not empty
		name := item.Keys[1].Token.Value().(string)
		if len(name) == 0 {
			return nil, errors.Errorf("backend name is empty at line %v, column %v",
				item.Pos().Line, item.Pos().Column)
		}

		// get the type of the backend by decoding it into the Backend truct
		var be Backend
		err := hcl.DecodeObject(&be, item)
		if err != nil {
			return nil, err
		}

		// then decode it into the right type
		var target interface{}
		switch be.Type {
		case "local", "":
			target = &BackendLocal{Type: "local"}
		case "sftp":
			target = &BackendSFTP{}
		default:
			return nil, errors.Errorf("backend type %q is unknown at line %v, column %v",
				be.Type, item.Pos().Line, item.Pos().Column)
		}

		err = hcl.DecodeObject(target, item)
		if err != nil {
			return nil, err
		}

		if _, ok := backends[name]; ok {
			return nil, errors.Errorf("backend %q at line %v, column %v already configured",
				name, item.Pos().Line, item.Pos().Column)
		}

		// check structure of the backend object
		innerBlock, ok := item.Val.(*ast.ObjectType)
		if !ok {
			return nil, errors.Errorf("unable to verify structure of backend %q at line %v, column %v already configured",
				name, item.Pos().Line, item.Pos().Column)
		}

		// check allowed types
		err = validateObjects(innerBlock.List, listTags(target, "hcl"))
		if err != nil {
			return nil, err
		}

		backends[name] = target
	}

	return backends, nil
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

	debug.Log("apply flags")

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

			slice := reflect.MakeSlice(reflect.TypeOf(v), len(v), len(v))
			field.Set(slice)

			for i, s := range v {
				slice.Index(i).SetString(s)
			}
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
