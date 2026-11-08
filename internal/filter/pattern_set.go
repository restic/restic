package filter

// minIndexedPatterns is the pattern count from which on indexing the patterns
// is faster than checking them one by one.
const minIndexedPatterns = 8

// patternSet is a list of patterns prepared for checking many paths. Patterns
// without wildcards are stored in tries of path components, so checking a path
// costs a few map lookups instead of one comparison per pattern. All other
// patterns are checked one by one.
type patternSet struct {
	absolute literalNode // wildcard-free patterns starting with "/"
	relative literalNode // wildcard-free patterns that can match at any depth
	others   []Pattern
}

type literalNode struct {
	children map[string]*literalNode
	terminal bool // a pattern ends here
}

func newPatternSet(patterns []Pattern) *patternSet {
	if len(patterns) < minIndexedPatterns {
		return &patternSet{others: patterns}
	}

	s := &patternSet{}
	for _, p := range patterns {
		if p.isNegated {
			// the order of the patterns matters, keep the plain list
			return &patternSet{others: patterns}
		}

		literal := true
		for _, part := range p.parts {
			// an empty part stands for "**", splitPath also returns it for
			// patterns like "/" or "C:\"
			if !part.isSimple || part.pattern == "" {
				literal = false
				break
			}
		}

		switch {
		case !literal:
			s.others = append(s.others, p)
		case p.parts[0].pattern == "/":
			s.absolute.insert(p.parts)
		default:
			s.relative.insert(p.parts)
		}
	}
	return s
}

func (n *literalNode) insert(parts []patternPart) {
	for _, part := range parts {
		if n.children == nil {
			n.children = make(map[string]*literalNode)
		}
		child, ok := n.children[part.pattern]
		if !ok {
			child = &literalNode{}
			n.children[part.pattern] = child
		}
		n = child
	}
	n.terminal = true
}

// matchPrefix reports whether a pattern equals strs[:i] for some i, and whether
// a pattern starts with strs, that is whether children of strs may match.
func (n *literalNode) matchPrefix(strs []string) (matched bool, childMayMatch bool) {
	for _, s := range strs {
		n = n.children[s]
		if n == nil {
			return false, false
		}
		if n.terminal {
			return true, true
		}
	}
	return false, true
}

// list returns the same result as list(patterns, checkChildMatches, str) for
// the patterns the set was created from. The only difference is that an error
// for a malformed pattern is not returned when a wildcard-free pattern matches.
func (s *patternSet) list(checkChildMatches bool, str string) (matched bool, childMayMatch bool, err error) {
	if s.absolute.children == nil && s.relative.children == nil {
		return list(s.others, checkChildMatches, str)
	}

	strs, err := prepareStr(str)
	if err != nil {
		return false, false, err
	}

	matched, childMayMatch = s.absolute.matchPrefix(strs)
	if s.relative.children != nil {
		// relative patterns can match at any depth and always match children.
		// A leading "/" never matches, no relative pattern starts with it.
		childMayMatch = true
		for i := 0; i < len(strs) && !matched; i++ {
			matched, _ = s.relative.matchPrefix(strs[i:])
		}
	}
	if !checkChildMatches {
		childMayMatch = true
	}
	if matched && childMayMatch {
		return true, true, nil
	}

	m, c, err := listStrs(s.others, checkChildMatches, strs)
	if err != nil {
		return false, false, err
	}
	return matched || m, childMayMatch || c, nil
}
