package autoscan

import (
	"fmt"
	"regexp"
)

// Filterer is a function that returns true if a path should be processed.
type Filterer func(string) bool

// NewFilterer compiles include and exclude patterns into a Filterer function.
// A path passes if it matches any include and no exclude. When includes is
// empty, all paths pass (subject to excludes).
func NewFilterer(includes, excludes []string) (Filterer, error) {
	reIncludes := make([]regexp.Regexp, 0)
	reExcludes := make([]regexp.Regexp, 0)

	// compile patterns
	for _, pattern := range includes {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("compiling include: %v: %w", pattern, err)
		}
		reIncludes = append(reIncludes, *re)
	}

	for _, pattern := range excludes {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("compiling exclude: %v: %w", pattern, err)
		}
		reExcludes = append(reExcludes, *re)
	}

	incSize := len(reIncludes)
	excSize := len(reExcludes)

	// create filterer
	var filter Filterer = func(string) bool { return true }

	if incSize > 0 || excSize > 0 {
		filter = func(path string) bool {
			// check excludes
			for _, re := range reExcludes {
				if re.MatchString(path) {
					return false
				}
			}

			// no includes (but excludes did not match)
			if incSize == 0 {
				return true
			}

			// check includes
			for _, re := range reIncludes {
				if re.MatchString(path) {
					return true
				}
			}

			// no includes passed
			return false
		}
	}

	return filter, nil
}
