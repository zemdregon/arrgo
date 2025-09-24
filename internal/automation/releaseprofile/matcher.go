package releaseprofile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type matchKind int

const (
	matchExact matchKind = iota
	matchContains
	matchRegex
)

// Matcher evaluates whether a release satisfies a condition.
type Matcher interface {
	Match(Release) bool
	Describe() string
}

type stringMatcher struct {
	field           Field
	kind            matchKind
	values          []string
	patterns        []*regexp.Regexp
	negate          bool
	caseInsensitive bool
}

func (m *stringMatcher) Match(release Release) bool {
	releaseValues := release.Values(m.field)
	matched := false

	if len(releaseValues) == 0 {
		matched = false
	} else {
		for _, releaseValue := range releaseValues {
			candidate := releaseValue
			if m.caseInsensitive {
				candidate = strings.ToLower(candidate)
			}
			switch m.kind {
			case matchExact:
				for _, value := range m.values {
					if candidate == value {
						matched = true
						break
					}
				}
			case matchContains:
				for _, value := range m.values {
					if strings.Contains(candidate, value) {
						matched = true
						break
					}
				}
			case matchRegex:
				for _, pattern := range m.patterns {
					if pattern.MatchString(releaseValue) {
						matched = true
						break
					}
				}
			}
			if matched {
				break
			}
		}
	}

	if m.negate {
		return !matched
	}
	return matched
}

func (m *stringMatcher) Describe() string {
	switch m.kind {
	case matchExact:
		return fmt.Sprintf("%s exact in %v (negate=%t)", m.field, m.values, m.negate)
	case matchContains:
		return fmt.Sprintf("%s contains %v (negate=%t)", m.field, m.values, m.negate)
	case matchRegex:
		patterns := make([]string, len(m.patterns))
		for i, pattern := range m.patterns {
			patterns[i] = pattern.String()
		}
		return fmt.Sprintf("%s regex in %v (negate=%t)", m.field, patterns, m.negate)
	default:
		return fmt.Sprintf("%s matcher (negate=%t)", m.field, m.negate)
	}
}

func newExactMatcher(field Field, values []string, negate bool) *stringMatcher {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		normalized = append(normalized, strings.ToLower(value))
	}
	return &stringMatcher{field: field, kind: matchExact, values: normalized, negate: negate, caseInsensitive: true}
}

func newContainsMatcher(field Field, values []string, negate bool) *stringMatcher {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		normalized = append(normalized, strings.ToLower(value))
	}
	return &stringMatcher{field: field, kind: matchContains, values: normalized, negate: negate, caseInsensitive: true}
}

func newRegexMatcher(field Field, patterns []string, negate bool) (*stringMatcher, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		expr := pattern
		if !strings.HasPrefix(pattern, "(?i)") {
			expr = "(?i)" + pattern
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("compile regex %q: %w", pattern, err)
		}
		compiled = append(compiled, re)
	}
	if len(compiled) == 0 {
		return nil, fmt.Errorf("no valid regex patterns provided")
	}
	return &stringMatcher{field: field, kind: matchRegex, patterns: compiled, negate: negate, caseInsensitive: true}, nil
}

type languageMatcher struct {
	language string
	except   bool
	negate   bool
}

func (m *languageMatcher) Match(release Release) bool {
	if len(release.Languages) == 0 {
		if m.negate {
			return true
		}
		return false
	}

	hasLanguage := containsFold(release.Languages, m.language)
	var matched bool
	if m.except {
		matched = false
		for _, lang := range release.Languages {
			if !strings.EqualFold(lang, m.language) {
				matched = true
				break
			}
		}
	} else {
		matched = hasLanguage
	}

	if m.negate {
		return !matched
	}
	return matched
}

func (m *languageMatcher) Describe() string {
	mode := "matches"
	if m.except {
		mode = "except"
	}
	return fmt.Sprintf("language %s %s (negate=%t)", mode, m.language, m.negate)
}

type sizeMatcher struct {
	minBytes float64
	maxBytes float64
	negate   bool
}

func (m *sizeMatcher) Match(release Release) bool {
	size := float64(release.SizeBytes)
	matched := size > m.minBytes && (m.maxBytes == 0 || size <= m.maxBytes)
	if m.negate {
		return !matched
	}
	return matched
}

func (m *sizeMatcher) Describe() string {
	return fmt.Sprintf("size between %.0f and %.0f (negate=%t)", m.minBytes, m.maxBytes, m.negate)
}

type numericMatcher struct {
	field  Field
	values []string
	negate bool
}

func (m *numericMatcher) Match(release Release) bool {
	releaseValues := release.Values(m.field)
	matched := false
	for _, candidate := range releaseValues {
		for _, value := range m.values {
			if candidate == value {
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if m.negate {
		return !matched
	}
	return matched
}

func (m *numericMatcher) Describe() string {
	return fmt.Sprintf("%s numeric in %v (negate=%t)", m.field, m.values, m.negate)
}

func toStringSlice(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}
