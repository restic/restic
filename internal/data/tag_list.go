package data

import (
	"fmt"
	"slices"
	"strings"

	"github.com/restic/restic/internal/ui"
)

// TagList is a list of tags.
type TagList []string

// splitTagList splits a string into a list of tags. The tags in the string
// need to be separated by commas. Whitespace is stripped around the individual
// tags. The empty tag only matches snapshots without any tags, so it is
// rejected unless it is the only tag in the list.
func splitTagList(s string) (TagList, error) {
	var l TagList
	for t := range strings.SplitSeq(s, ",") {
		l = append(l, strings.TrimSpace(t))
	}
	if len(l) > 1 && slices.Contains(l, "") {
		return nil, fmt.Errorf("invalid tag list %q, the empty tag must be the only tag in the list", s)
	}
	return l, nil
}

func (l TagList) String() string {
	return "[" + strings.Join(ui.QuoteSlice(l), ", ") + "]"
}

// Set updates the TagList's value.
func (l *TagList) Set(s string) error {
	tags, err := splitTagList(s)
	if err != nil {
		return err
	}
	*l = tags
	return nil
}

// Type returns a description of the type.
func (TagList) Type() string {
	return "TagList"
}

// TagLists consists of several TagList.
type TagLists []TagList

func (l TagLists) String() string {
	return fmt.Sprint([]TagList(l))
}

// Flatten returns the list of all tags provided in TagLists
func (l TagLists) Flatten() (tags TagList) {
	tags = make([]string, 0)
	for _, list := range l {
		for _, tag := range list {
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	return tags
}

// Set updates the TagList's value.
func (l *TagLists) Set(s string) error {
	tags, err := splitTagList(s)
	if err != nil {
		return err
	}
	*l = append(*l, tags)
	return nil
}

// Type returns a description of the type.
func (TagLists) Type() string {
	return "TagLists"
}
