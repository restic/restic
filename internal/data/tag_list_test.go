package data

import (
	"strconv"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestTagLists_Flatten(t *testing.T) {
	tests := []struct {
		name string
		l    TagLists
		want TagList
	}{
		{
			name: "4 tags",
			l: TagLists{
				TagList{
					"tag1",
					"tag2",
				},
				TagList{
					"tag3",
					"tag4",
				},
			},
			want: TagList{"tag1", "tag2", "tag3", "tag4"},
		},
		{
			name: "No tags",
			l:    nil,
			want: TagList{},
		},
		{
			name: "Empty tags",
			l:    TagLists{[]string{""}},
			want: TagList{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.l.Flatten()
			rtest.Equals(t, got, tt.want)
		})
	}
}

func TestTagListSet(t *testing.T) {
	for _, test := range []struct {
		value string
		want  TagList
	}{
		{"", TagList{""}},
		{"foo", TagList{"foo"}},
		{"foo,bar", TagList{"foo", "bar"}},
		{" foo , bar ", TagList{"foo", "bar"}},
	} {
		var l TagList
		rtest.OK(t, l.Set(test.value))
		rtest.Equals(t, test.want, l)

		var ls TagLists
		rtest.OK(t, ls.Set(test.value))
		rtest.Equals(t, TagLists{test.want}, ls)
	}
}

func TestTagListSetInvalid(t *testing.T) {
	// the empty tag only matches snapshots without any tags, so a list that
	// mixes it with another tag can never match and is rejected
	for _, value := range []string{",", ",foo", "foo,", "foo,,bar", "foo, ,bar"} {
		var l TagList
		rtest.Assert(t, l.Set(value) != nil, "missing error for tag list %q", value)
		rtest.Equals(t, TagList(nil), l)

		var ls TagLists
		rtest.Assert(t, ls.Set(value) != nil, "missing error for tag list %q", value)
		rtest.Equals(t, TagLists(nil), ls)
	}
}

func TestTagListStringQuotes(t *testing.T) {
	rtest.Equals(t, "[ok, plain]", TagList{"ok", "plain"}.String())
	rtest.Equals(t, "[ok, "+strconv.Quote("a\nb")+"]", TagList{"ok", "a\nb"}.String())
}
