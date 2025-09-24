package releaseprofile

import "strings"

// Field enumerates attributes available for release matching.
type Field string

// Supported release metadata fields.
const (
	FieldTitle       Field = "title"
	FieldGroup       Field = "release_group"
	FieldSource      Field = "source"
	FieldResolution  Field = "resolution"
	FieldVideoCodec  Field = "video_codec"
	FieldAudioCodec  Field = "audio_codec"
	FieldContainer   Field = "container"
	FieldEdition     Field = "edition"
	FieldLanguage    Field = "language"
	FieldIndexerFlag Field = "indexer_flag"
	FieldReleaseType Field = "release_type"
	FieldQualityMod  Field = "quality_modifier"
	FieldTag         Field = "tag"
	FieldCustom      Field = "custom"
)

// Release models information about an indexer or download candidate that can
// be evaluated against custom formats and release profiles.
type Release struct {
	Title        string
	Group        string
	Source       string
	Resolution   string
	VideoCodec   string
	AudioCodec   string
	Container    string
	Edition      string
	ReleaseType  string
	QualityMod   string
	Tags         []string
	Languages    []string
	IndexerFlags []string
	SizeBytes    int64

	Extra map[Field][]string
}

// Values returns the release metadata stored under the given field name.
func (r Release) Values(field Field) []string {
	switch field {
	case FieldTitle:
		if r.Title != "" {
			return []string{r.Title}
		}
	case FieldGroup:
		if r.Group != "" {
			return []string{r.Group}
		}
	case FieldSource:
		if r.Source != "" {
			return []string{r.Source}
		}
	case FieldResolution:
		if r.Resolution != "" {
			return []string{r.Resolution}
		}
	case FieldVideoCodec:
		if r.VideoCodec != "" {
			return []string{r.VideoCodec}
		}
	case FieldAudioCodec:
		if r.AudioCodec != "" {
			return []string{r.AudioCodec}
		}
	case FieldContainer:
		if r.Container != "" {
			return []string{r.Container}
		}
	case FieldEdition:
		if r.Edition != "" {
			return []string{r.Edition}
		}
	case FieldReleaseType:
		if r.ReleaseType != "" {
			return []string{r.ReleaseType}
		}
	case FieldQualityMod:
		if r.QualityMod != "" {
			return []string{r.QualityMod}
		}
	case FieldLanguage:
		if len(r.Languages) > 0 {
			return r.Languages
		}
	case FieldIndexerFlag:
		if len(r.IndexerFlags) > 0 {
			return r.IndexerFlags
		}
	case FieldTag:
		if len(r.Tags) > 0 {
			return r.Tags
		}
	}

	if r.Extra != nil {
		if values, ok := r.Extra[field]; ok {
			return values
		}
	}

	return nil
}

// AddValues stores arbitrary metadata values for the provided field.
func (r *Release) AddValues(field Field, values ...string) {
	if r.Extra == nil {
		r.Extra = make(map[Field][]string)
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return
	}
	r.Extra[field] = append(r.Extra[field], normalized...)
}

// containsFold reports whether the provided slice contains the given value
// using case-insensitive comparison.
func containsFold(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if strings.EqualFold(candidate, needle) {
			return true
		}
	}
	return false
}
