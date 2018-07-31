// Copyright 2018, Google
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pyre

import (
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

type testVersionedObject struct {
	name     string
	versions []string
}

func (t testVersionedObject) Name() string { return t.name }

func (t testVersionedObject) NextNVersions(b string, n int) ([]string, error) {
	var out []string
	var seen bool
	if b == "" {
		seen = true
	}
	for _, v := range t.versions {
		if b == v {
			seen = true
		}
		if !seen {
			continue
		}
		if len(out) >= n {
			return out, nil
		}
		out = append(out, v)
	}
	return out, nil
}

type testListManager struct {
	objs map[string][]string
	m    sync.Mutex
}

func (t *testListManager) NextN(b, fn, pfx, spfx string, n int) ([]VersionedObject, error) {
	t.m.Lock()
	defer t.m.Unlock()

	var out []VersionedObject
	var keys []string
	for k := range t.objs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k < fn {
			continue
		}
		if !strings.HasPrefix(k, pfx) {
			continue
		}
		if spfx != "" && strings.HasPrefix(k, spfx) {
			continue
		}
		out = append(out, testVersionedObject{name: k, versions: t.objs[k]})
		n--
		if n <= 0 {
			return out, nil
		}
	}
	return out, nil
}

func TestGetDirNames(t *testing.T) {
	table := []struct {
		lm    ListManager
		name  string
		pfx   string
		delim string
		num   int
		want  []string
	}{
		{
			lm: &testListManager{
				objs: map[string][]string{
					"/usr/local/etc/foo/bar": {"a"},
					"/usr/local/etc/foo/baz": {"a"},
					"/usr/local/etc/foo":     {"a"},
					"/usr/local/etc/fool":    {"a"},
				},
			},
			num:   2,
			pfx:   "/usr/local/etc/",
			delim: "/",
			want:  []string{"/usr/local/etc/foo", "/usr/local/etc/foo/"},
		},
		{
			lm: &testListManager{
				objs: map[string][]string{
					"/usr/local/etc/foo/bar": {"a"},
					"/usr/local/etc/foo/baz": {"a"},
					"/usr/local/etc/foo":     {"a"},
					"/usr/local/etc/fool":    {"a"},
					"/usr/local/etc/bar":     {"a"},
				},
			},
			num:   4,
			pfx:   "/usr/local/etc/",
			delim: "/",
			want:  []string{"/usr/local/etc/bar", "/usr/local/etc/foo", "/usr/local/etc/foo/", "/usr/local/etc/fool"},
		},
	}

	for _, e := range table {
		got, err := getDirNames(e.lm, "", e.name, e.pfx, e.delim, e.num)
		if err != nil {
			t.Error(err)
			continue
		}
		if !reflect.DeepEqual(got, e.want) {
			t.Errorf("getDirNames(%v, %q, %q, %q, %d): got %v, want %v", e.lm, e.name, e.pfx, e.delim, e.num, got, e.want)
		}
	}
}
