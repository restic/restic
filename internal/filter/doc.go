// Package filter implements path filters similar to filepath.Match, but
// patterns may span multiple path components and use a recursive "**" wildcard.
//
// Single-component wildcards follow filepath.Match rules; see Match for details.
package filter
