package test

import (
	"fmt"
	"reflect"
	"testing"
)

type ConfigTestData[C comparable] struct {
	S   string
	Cfg C
}

func ParseConfigTester[C comparable](t *testing.T, parser func(s string) (*C, error), tests []ConfigTestData[C]) {
	for i, test := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			cfg, err := parser(test.S)
			if err != nil {
				t.Fatalf("%s failed: %v", test.S, err)
			}

			if !reflect.DeepEqual(*cfg, test.Cfg) {
				t.Fatalf("input: %s\n wrong config, want:\n  %#v\ngot:\n  %#v",
					test.S, test.Cfg, *cfg)
			}
		})
	}
}
