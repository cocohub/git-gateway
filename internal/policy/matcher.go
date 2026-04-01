package policy

import (
	"path/filepath"
	"strings"
)

// MatchGlob checks if a string matches a glob pattern.
// Supports * (single segment) and ** (multiple segments).
func MatchGlob(pattern, s string) bool {
	// Handle ** patterns by converting to a regex-like match
	if strings.Contains(pattern, "**") {
		return matchDoublestar(pattern, s)
	}
	// Use filepath.Match for simple * patterns
	matched, _ := filepath.Match(pattern, s)
	return matched
}

// matchDoublestar handles ** glob patterns.
func matchDoublestar(pattern, s string) bool {
	// Split pattern and string by /
	patternParts := strings.Split(pattern, "/")
	stringParts := strings.Split(s, "/")

	return matchParts(patternParts, stringParts)
}

func matchParts(pattern, s []string) bool {
	pi, si := 0, 0

	for pi < len(pattern) && si < len(s) {
		if pattern[pi] == "**" {
			// ** matches zero or more path segments
			if pi == len(pattern)-1 {
				// ** at end matches everything
				return true
			}
			// Try matching ** with different numbers of segments
			for skip := 0; skip <= len(s)-si; skip++ {
				if matchParts(pattern[pi+1:], s[si+skip:]) {
					return true
				}
			}
			return false
		}

		// Match current segment with potential wildcards
		matched, _ := filepath.Match(pattern[pi], s[si])
		if !matched {
			return false
		}
		pi++
		si++
	}

	// Handle trailing ** in pattern
	for pi < len(pattern) && pattern[pi] == "**" {
		pi++
	}

	return pi == len(pattern) && si == len(s)
}

// MatchAny returns true if s matches any of the patterns.
func MatchAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if MatchGlob(p, s) {
			return true
		}
	}
	return false
}
