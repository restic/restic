package restic

import (
	rtest "github.com/restic/restic/internal/test"
	"testing"
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
