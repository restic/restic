package restic

import (
	"fmt"
	"strings"
)

// TagList is a list of tags.
type TagList []string

// splitTagList splits a string into a list of tags. The tags in the string
// need to be separated by commas. Whitespace is stripped around the individual
// tags.
func splitTagList(s string) (l TagList) {
	for _, t := range strings.Split(s, ",") {
		l = append(l, strings.TrimSpace(t))
	}
	return l
}

func (l TagList) String() string {
	return "[" + strings.Join(l, ", ") + "]"
}

// Set updates the TagList's value.
func (l *TagList) Set(s string) error {
	*l = splitTagList(s)
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
	*l = append(*l, splitTagList(s))
	return nil
}

// Type returns a description of the type.
func (TagLists) Type() string {
	return "TagLists"
}
