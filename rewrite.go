package autoscan

import (
	"fmt"
	"regexp"
)

// Rewrite is a rule that rewrites a path from one pattern to another.
type Rewrite struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// Rewriter is a function that rewrites a path string.
type Rewriter func(string) string

// NewRewriter compiles a slice of Rewrite rules into a Rewriter function.
func NewRewriter(rewriteRules []Rewrite) (Rewriter, error) {
	var rewrites []regexp.Regexp
	for _, rule := range rewriteRules {
		re, err := regexp.Compile(rule.From)
		if err != nil {
			return nil, fmt.Errorf("compiling rewrite from %q: %w", rule.From, err)
		}

		rewrites = append(rewrites, *re)
	}

	rewriter := func(input string) string {
		for i, r := range rewrites {
			if r.MatchString(input) {
				return r.ReplaceAllString(input, rewriteRules[i].To)
			}
		}

		return input
	}

	return rewriter, nil
}
