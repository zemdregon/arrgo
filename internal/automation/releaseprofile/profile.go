package releaseprofile

import "errors"

// ErrReleaseBlocked indicates that a release matched a blocking rule in the
// profile and should not be grabbed automatically.
var ErrReleaseBlocked = errors.New("release blocked by profile")

// CustomFormat encapsulates a reusable set of matchers mirroring the custom
// format concept from the *arr applications.
type CustomFormat struct {
	ID                  int
	Name                string
	TrashID             string
	TrashDescription    string
	IncludeWhenRenaming bool
	Matchers            []Matcher
}

// Matches reports whether the release satisfies every matcher associated with
// the custom format.
func (f CustomFormat) Matches(release Release) bool {
	for _, matcher := range f.Matchers {
		if !matcher.Match(release) {
			return false
		}
	}
	return true
}

// FormatScore couples a custom format with a weight used during scoring.
type FormatScore struct {
	Format CustomFormat
	Score  float64
}

// Preference represents an additive score awarded when a matcher is satisfied.
type Preference struct {
	Name    string
	Matcher Matcher
	Score   float64
}

// BlockRule prevents releases that satisfy the matchers from being considered.
type BlockRule struct {
	Name     string
	Matchers []Matcher
}

// Matches reports whether the release satisfies every matcher on the block
// rule.
func (b BlockRule) Matches(release Release) bool {
	for _, matcher := range b.Matchers {
		if !matcher.Match(release) {
			return false
		}
	}
	return true
}

// Profile coordinates weighted scoring, preferences, and blocklists for a
// specific automation policy.
type Profile struct {
	Name         string
	Description  string
	DefaultScore float64

	MinFormatScore        float64
	CutoffFormatScore     float64
	MinUpgradeFormatScore float64

	Formats     []FormatScore
	Preferences []Preference
	Blocklist   []BlockRule
}

// Score evaluates a release against the profile, returning the aggregated
// score or an error when the release is blocked.
func (p Profile) Score(release Release) (float64, error) {
	for _, block := range p.Blocklist {
		if block.Matches(release) {
			return 0, ErrReleaseBlocked
		}
	}

	total := p.DefaultScore
	for _, pref := range p.Preferences {
		if pref.Matcher.Match(release) {
			total += pref.Score
		}
	}

	for _, format := range p.Formats {
		if format.Format.Matches(release) {
			total += format.Score
		}
	}

	return total, nil
}

// NewGroupPreference constructs a preference that applies when the release
// group matches any of the supplied patterns.
func NewGroupPreference(name string, patterns []string, score float64, negate bool) (Preference, error) {
	matcher, err := newRegexMatcher(FieldGroup, patterns, negate)
	if err != nil {
		return Preference{}, err
	}
	return Preference{Name: name, Matcher: matcher, Score: score}, nil
}

// NewCodecPreference constructs a preference that applies when the release
// codec matches any of the supplied strings.
func NewCodecPreference(name string, codecs []string, score float64, isVideo bool, negate bool) Preference {
	field := FieldAudioCodec
	if isVideo {
		field = FieldVideoCodec
	}
	matcher := newExactMatcher(field, codecs, negate)
	return Preference{Name: name, Matcher: matcher, Score: score}
}

// NewTitlePreference constructs a preference that evaluates release titles via
// regular expressions.
func NewTitlePreference(name string, patterns []string, score float64, negate bool) (Preference, error) {
	matcher, err := newRegexMatcher(FieldTitle, patterns, negate)
	if err != nil {
		return Preference{}, err
	}
	return Preference{Name: name, Matcher: matcher, Score: score}, nil
}
