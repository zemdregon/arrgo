package releaseprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type servarrCustomFormat struct {
	ID                  int                    `json:"id"`
	TrashID             string                 `json:"trash_id"`
	TrashDescription    string                 `json:"trash_description"`
	Name                string                 `json:"name"`
	IncludeWhenRenaming bool                   `json:"includeCustomFormatWhenRenaming"`
	Specifications      []servarrSpecification `json:"specifications"`
}

type servarrSpecification struct {
	Name           string    `json:"name"`
	Implementation string    `json:"implementation"`
	Negate         bool      `json:"negate"`
	Required       bool      `json:"required"`
	Fields         fieldsMap `json:"fields"`
}

type fieldsMap map[string]any

func (f *fieldsMap) UnmarshalJSON(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 || string(bytes.TrimSpace(data)) == "null" {
		*f = make(fieldsMap)
		return nil
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err == nil {
		*f = obj
		return nil
	}

	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err == nil {
		converted := make(map[string]any, len(arr))
		for _, entry := range arr {
			rawName, ok := entry["name"].(string)
			if !ok || rawName == "" {
				continue
			}
			converted[rawName] = entry["value"]
		}
		*f = converted
		return nil
	}

	return fmt.Errorf("unsupported fields payload: %s", string(data))
}

func (f fieldsMap) String(key string) (string, bool) {
	value, ok := f[key]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case int:
		return strconv.Itoa(typed), true
	case json.Number:
		return typed.String(), true
	case bool:
		if typed {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

func (f fieldsMap) Int(key string) (int, bool) {
	value, ok := f[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func (f fieldsMap) Float(key string) (float64, bool) {
	value, ok := f[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func (f fieldsMap) Bool(key string) bool {
	value, ok := f[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err != nil {
			return false
		}
		return parsed
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}

// ImportSonarrCustomFormats converts Sonarr custom format definitions into the
// reusable arrgo representation.
func ImportSonarrCustomFormats(data []byte) ([]CustomFormat, error) {
	return importServarrCustomFormats(data)
}

// ImportRadarrCustomFormats converts Radarr custom format definitions into the
// reusable arrgo representation.
func ImportRadarrCustomFormats(data []byte) ([]CustomFormat, error) {
	return importServarrCustomFormats(data)
}

func importServarrCustomFormats(data []byte) ([]CustomFormat, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()

	formats := make([]servarrCustomFormat, 0)
	if trimmed[0] == '[' {
		if err := decoder.Decode(&formats); err != nil {
			return nil, fmt.Errorf("decode custom formats: %w", err)
		}
	} else {
		var single servarrCustomFormat
		if err := decoder.Decode(&single); err != nil {
			return nil, fmt.Errorf("decode custom format: %w", err)
		}
		formats = append(formats, single)
	}

	result := make([]CustomFormat, 0, len(formats))
	for _, item := range formats {
		cf, err := convertCustomFormat(item)
		if err != nil {
			return nil, fmt.Errorf("custom format %q: %w", item.Name, err)
		}
		result = append(result, cf)
	}

	return result, nil
}

func convertCustomFormat(format servarrCustomFormat) (CustomFormat, error) {
	matchers := make([]Matcher, 0, len(format.Specifications))
	for _, spec := range format.Specifications {
		matcher, err := convertSpecification(spec)
		if err != nil {
			return CustomFormat{}, fmt.Errorf("specification %q: %w", spec.Name, err)
		}
		if matcher != nil {
			matchers = append(matchers, matcher)
		}
	}

	return CustomFormat{
		ID:                  format.ID,
		Name:                format.Name,
		TrashID:             format.TrashID,
		TrashDescription:    format.TrashDescription,
		IncludeWhenRenaming: format.IncludeWhenRenaming,
		Matchers:            matchers,
	}, nil
}

func convertSpecification(spec servarrSpecification) (Matcher, error) {
	switch spec.Implementation {
	case "ReleaseGroupSpecification":
		pattern, ok := spec.Fields.String("value")
		if !ok {
			return nil, fmt.Errorf("release group specification missing value")
		}
		return newRegexMatcher(FieldGroup, []string{pattern}, spec.Negate)
	case "ReleaseTitleSpecification":
		pattern, ok := spec.Fields.String("value")
		if !ok {
			return nil, fmt.Errorf("release title specification missing value")
		}
		return newRegexMatcher(FieldTitle, []string{pattern}, spec.Negate)
	case "EditionSpecification":
		pattern, ok := spec.Fields.String("value")
		if !ok {
			return nil, fmt.Errorf("edition specification missing value")
		}
		return newRegexMatcher(FieldEdition, []string{pattern}, spec.Negate)
	case "SourceSpecification":
		id, ok := spec.Fields.Int("value")
		if !ok {
			return nil, fmt.Errorf("source specification missing value")
		}
		source, ok := qualitySourceByID[id]
		if !ok {
			return nil, fmt.Errorf("unknown source id %d", id)
		}
		return newExactMatcher(FieldSource, []string{source}, spec.Negate), nil
	case "ResolutionSpecification":
		value, ok := spec.Fields.Int("value")
		if !ok {
			return nil, fmt.Errorf("resolution specification missing value")
		}
		return newExactMatcher(FieldResolution, resolutionValues(value), spec.Negate), nil
	case "LanguageSpecification":
		id, ok := spec.Fields.Int("value")
		if !ok {
			return nil, fmt.Errorf("language specification missing value")
		}
		language, ok := languageByID[id]
		if !ok {
			return nil, fmt.Errorf("unknown language id %d", id)
		}
		matcher := &languageMatcher{language: language, except: spec.Fields.Bool("exceptLanguage"), negate: spec.Negate}
		return matcher, nil
	case "IndexerFlagSpecification":
		id, ok := spec.Fields.Int("value")
		if !ok {
			return nil, fmt.Errorf("indexer flag specification missing value")
		}
		flag, ok := indexerFlagByID[id]
		if !ok {
			return nil, fmt.Errorf("unknown indexer flag id %d", id)
		}
		return newExactMatcher(FieldIndexerFlag, []string{flag}, spec.Negate), nil
	case "ReleaseTypeSpecification":
		id, ok := spec.Fields.Int("value")
		if !ok {
			return nil, fmt.Errorf("release type specification missing value")
		}
		releaseType, ok := releaseTypeByID[id]
		if !ok {
			return nil, fmt.Errorf("unknown release type id %d", id)
		}
		return newExactMatcher(FieldReleaseType, []string{releaseType}, spec.Negate), nil
	case "QualityModifierSpecification":
		id, ok := spec.Fields.Int("value")
		if !ok {
			return nil, fmt.Errorf("quality modifier specification missing value")
		}
		modifier, ok := qualityModifierByID[id]
		if !ok {
			return nil, fmt.Errorf("unknown quality modifier id %d", id)
		}
		return newExactMatcher(FieldQualityMod, []string{modifier}, spec.Negate), nil
	case "SizeSpecification":
		min, _ := spec.Fields.Float("min")
		max, _ := spec.Fields.Float("max")
		minBytes := math.Max(0, min) * bytesPerGigabyte
		maxBytes := 0.0
		if max > 0 {
			maxBytes = max * bytesPerGigabyte
		}
		return &sizeMatcher{minBytes: minBytes, maxBytes: maxBytes, negate: spec.Negate}, nil
	default:
		return nil, fmt.Errorf("unsupported specification implementation %q", spec.Implementation)
	}
}

const bytesPerGigabyte = 1024 * 1024 * 1024

var qualitySourceByID = map[int]string{
	0: "unknown",
	1: "television",
	2: "television_raw",
	3: "web",
	4: "webrip",
	5: "dvd",
	6: "bluray",
	7: "bluray_raw",
}

func resolutionValues(resolution int) []string {
	if resolution <= 0 {
		return []string{"unknown"}
	}
	base := strconv.Itoa(resolution)
	return []string{base, base + "p"}
}

var indexerFlagByID = map[int]string{
	1:   "freeleech",
	2:   "halfleech",
	4:   "double_upload",
	8:   "internal",
	16:  "scene",
	32:  "freeleech75",
	64:  "freeleech25",
	128: "nuked",
}

var releaseTypeByID = map[int]string{
	0: "unknown",
	1: "single_episode",
	2: "multi_episode",
	3: "season_pack",
}

var qualityModifierByID = map[int]string{
	0: "none",
	1: "regional",
	2: "screener",
	3: "rawhd",
	4: "brdisk",
	5: "remux",
}

var languageByID = map[int]string{
	0:  "Unknown",
	1:  "English",
	2:  "French",
	3:  "Spanish",
	4:  "German",
	5:  "Italian",
	6:  "Danish",
	7:  "Dutch",
	8:  "Japanese",
	9:  "Icelandic",
	10: "Chinese",
	11: "Russian",
	12: "Polish",
	13: "Vietnamese",
	14: "Swedish",
	15: "Norwegian",
	16: "Finnish",
	17: "Turkish",
	18: "Portuguese",
	19: "Flemish",
	20: "Greek",
	21: "Korean",
	22: "Hungarian",
	23: "Hebrew",
	24: "Lithuanian",
	25: "Czech",
	26: "Arabic",
	27: "Hindi",
	28: "Bulgarian",
	29: "Malayalam",
	30: "Ukrainian",
	31: "Slovak",
	32: "Thai",
	33: "Portuguese (Brazil)",
	34: "Spanish (Latino)",
	35: "Romanian",
	36: "Latvian",
	37: "Persian",
	38: "Catalan",
	39: "Croatian",
	40: "Serbian",
	41: "Bosnian",
	42: "Estonian",
	43: "Tamil",
	44: "Indonesian",
	45: "Macedonian",
	46: "Slovenian",
	-2: "Original",
}

// ImportSonarrQualityProfile converts a Sonarr quality profile JSON payload into
// an arrgo release profile. Custom formats referenced by the profile must be
// provided so the importer can attach scoring weights.
type sonarrQualityProfile struct {
	Name                  string                    `json:"name"`
	MinFormatScore        float64                   `json:"minFormatScore"`
	CutoffFormatScore     float64                   `json:"cutoffFormatScore"`
	MinUpgradeFormatScore float64                   `json:"minUpgradeFormatScore"`
	FormatItems           []sonarrProfileFormatItem `json:"formatItems"`
}

type sonarrProfileFormatItem struct {
	FormatID int     `json:"format"`
	Name     string  `json:"name"`
	Score    float64 `json:"score"`
}

// ImportSonarrQualityProfile builds a release profile from a Sonarr quality
// profile definition and a set of custom formats.
func ImportSonarrQualityProfile(data []byte, formats []CustomFormat) (Profile, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Profile{}, fmt.Errorf("quality profile payload is empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()

	var profileResource sonarrQualityProfile
	if err := decoder.Decode(&profileResource); err != nil {
		return Profile{}, fmt.Errorf("decode quality profile: %w", err)
	}

	formatByID := make(map[int]CustomFormat, len(formats))
	trashByID := make(map[string]CustomFormat, len(formats))
	nameByID := make(map[string]CustomFormat, len(formats))
	for _, format := range formats {
		if format.ID != 0 {
			formatByID[format.ID] = format
		}
		if format.TrashID != "" {
			trashByID[strings.ToLower(format.TrashID)] = format
		}
		if format.Name != "" {
			nameByID[strings.ToLower(format.Name)] = format
		}
	}

	profile := Profile{
		Name:                  profileResource.Name,
		MinFormatScore:        profileResource.MinFormatScore,
		CutoffFormatScore:     profileResource.CutoffFormatScore,
		MinUpgradeFormatScore: profileResource.MinUpgradeFormatScore,
	}

	for _, item := range profileResource.FormatItems {
		format, ok := formatByID[item.FormatID]
		if !ok {
			lowered := strings.ToLower(item.Name)
			if cf, exists := nameByID[lowered]; exists {
				format = cf
				ok = true
			} else if cf, exists := trashByID[lowered]; exists {
				format = cf
				ok = true
			}
		}
		if !ok {
			return Profile{}, fmt.Errorf("unknown custom format id %d referenced by profile", item.FormatID)
		}
		profile.Formats = append(profile.Formats, FormatScore{Format: format, Score: item.Score})
	}

	return profile, nil
}
