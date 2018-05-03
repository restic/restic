package config

import (
	"fmt"
	"reflect"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/hashicorp/hcl/hcl/token"
	"github.com/restic/restic/internal/errors"
)

// Repo is a configured repository
type Repo struct {
	Backend string
	Path    string
}

// Config contains configuration items read from a file.
type Config struct {
	Quiet bool            `hcl:"quiet"`
	Repos map[string]Repo `hcl:"repo"`
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

	// check for additional top-level items
	validNames := listTags(cfg, "hcl")
	for _, item := range parsed.Node.(*ast.ObjectList).Items {
		fmt.Printf("-----------\n")
		spew.Dump(item)
		var ident string
		for _, key := range item.Keys {
			if key.Token.Type == token.IDENT {
				ident = key.Token.Text
			}
		}
		fmt.Printf("ident is %q\n", ident)

		if _, ok := validNames[ident]; !ok {
			return Config{}, errors.Errorf("unknown option %q found at line %v, column %v: %v",
				ident, item.Pos().Line, item.Pos().Column)
		}
	}
	// spew.Dump(cfg)

	return cfg, nil
}
